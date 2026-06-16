<template>
  <div class="run-history">
    <div class="history-header">
      <div>
        <div class="breadcrumb">
          <button type="button" class="breadcrumb-link" @click="$emit('back')">评测集</button>
          <span class="breadcrumb-separator">›</span>
          <button type="button" class="breadcrumb-link current-link" @click="$emit('samples')">{{ dataset.name }}</button>
          <span class="breadcrumb-separator">›</span>
          <span class="breadcrumb-current">历史记录</span>
        </div>
        <p class="history-subtitle">仅显示当前评测集的运行记录。</p>
      </div>
      <div class="history-actions">
        <t-button variant="outline" @click="$emit('samples')">返回样本</t-button>
        <t-button theme="primary" @click="$emit('create-run')">创建运行</t-button>
      </div>
    </div>

    <section class="history-panel">
      <div class="compare-controls">
        <t-select v-model="baselineRunId" placeholder="选择基线运行" filterable>
          <t-option v-for="run in completedRuns" :key="run.id" :value="run.id" :label="runLabel(run)" />
        </t-select>
        <span class="compare-arrow">→</span>
        <t-select v-model="candidateRunId" placeholder="选择候选运行" filterable>
          <t-option v-for="run in completedRuns" :key="run.id" :value="run.id" :label="runLabel(run)" />
        </t-select>
        <t-button :loading="comparing" @click="handleCompare">开始对比</t-button>
      </div>

      <t-table row-key="id" :data="runs" :columns="runColumns" :loading="loading" hover @row-click="openRun">
        <template #status="{ row }">
          <t-tag :theme="statusTheme(row.status)" variant="light">{{ statusText(row.status) }}</t-tag>
        </template>
        <template #progress="{ row }">
          <t-progress size="small" :percentage="progress(row)" :label="`${row.finished_samples}/${row.total_samples}`" />
        </template>
        <template #metrics="{ row }">
          <span>{{ Object.keys(row.aggregate_metric_scores || {}).length }} 项</span>
        </template>
        <template #operation="{ row }">
          <t-link theme="primary" @click.stop="$emit('open-run', row)">查看详情</t-link>
        </template>
      </t-table>

      <div v-if="comparison" class="comparison-panel">
        <h2>运行对比</h2>
        <t-table row-key="name" :data="comparison.metrics" :columns="comparisonColumns">
          <template #metric="{ row }">
            <strong>{{ row.name }}</strong><span class="metric-version">{{ row.version }}</span>
          </template>
          <template #score="{ row }">
            {{ formatScore(row.baseline_score) }} → {{ formatScore(row.candidate_score) }}
          </template>
          <template #delta="{ row }">
            <span :class="row.improved ? 'improved' : row.delta < 0 ? 'declined' : ''">{{ signedScore(row.delta) }}</span>
          </template>
          <template #improved="{ row }">
            <t-tag :theme="row.improved ? 'success' : 'default'" variant="light">{{ row.improved ? '改善' : '未改善' }}</t-tag>
          </template>
        </t-table>
      </div>
    </section>

    <t-drawer v-model:visible="drawerVisible" size="82%" :header="selectedRun ? `运行详情 · ${selectedRun.dataset_name}` : '运行详情'">
      <template v-if="selectedRun">
        <div class="run-summary">
          <div><span>状态</span><t-tag :theme="statusTheme(selectedRun.status)" variant="light">{{ statusText(selectedRun.status) }}</t-tag></div>
          <div><span>进度</span><strong>{{ selectedRun.finished_samples }}/{{ selectedRun.total_samples }}</strong></div>
          <div><span>失败样本</span><strong>{{ selectedRun.failed_samples }}</strong></div>
          <div><span>运行 ID</span><code>{{ selectedRun.id }}</code></div>
        </div>
        <t-alert v-if="selectedRun.error" theme="error" :message="selectedRun.error" />
        <details class="snapshot">
          <summary>配置快照</summary>
          <pre>{{ JSON.stringify(selectedRun.config_snapshot, null, 2) }}</pre>
        </details>
        <h3>聚合指标</h3>
        <div class="metric-grid">
          <div v-for="score in metricScoreList(selectedRun.aggregate_metric_scores)" :key="score.name" class="metric-card">
            <div>{{ score.name }} <small>{{ score.version }}</small></div>
            <strong>{{ score.score == null ? '—' : formatScore(score.score) }}</strong>
            <span>{{ score.scored_sample_count || 0 }}/{{ score.total_sample_count || selectedRun.total_samples }} 个样本</span>
          </div>
        </div>
        <h3>样本结果</h3>
        <t-table row-key="id" :data="runResults" :columns="resultColumns" :loading="loadingRunResults" table-layout="fixed" @row-click="selectResult">
          <template #status="{ row }">
            <t-tag :theme="row.status === 'completed' ? 'success' : row.status === 'failed' ? 'danger' : 'default'" variant="light">{{ row.status }}</t-tag>
          </template>
          <template #answer="{ row }">
            <div class="clamp">{{ row.generated_answer || row.error || '—' }}</div>
          </template>
          <template #metrics="{ row }">
            {{ Object.values(row.metric_scores || {}).filter((metric: any) => metric.status === 'scored').length }}
          </template>
        </t-table>
        <div v-if="selectedResult" class="result-detail">
          <h3>样本证据</h3>
          <div class="evidence-grid">
            <div>
              <h4>问题</h4>
              <p>{{ selectedResult.question }}</p>
              <h4>参考答案</h4>
              <p>{{ selectedResult.reference_answer }}</p>
              <h4>生成答案</h4>
              <p>{{ selectedResult.generated_answer || selectedResult.error }}</p>
            </div>
            <div>
              <h4>检索上下文</h4>
              <ol>
                <li v-for="context in selectedResult.retrieved_contexts" :key="`${context.rank}-${context.chunk_id}`">
                  <span class="muted">#{{ context.rank }} · {{ formatScore(context.score) }}</span>
                  <p>{{ context.text }}</p>
                </li>
              </ol>
            </div>
          </div>
        </div>
      </template>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import type { EvaluationDataset, EvaluationRun, EvaluationRunResult, MetricScore, RunComparison } from '@/api/evaluation'

const props = defineProps<{
  dataset: EvaluationDataset
  runs: EvaluationRun[]
  runResults: EvaluationRunResult[]
  selectedRun: EvaluationRun | null
  comparison: RunComparison | null
  loading: boolean
  loadingRunResults: boolean
  comparing: boolean
}>()

const emit = defineEmits<{
  (event: 'back'): void
  (event: 'samples'): void
  (event: 'create-run'): void
  (event: 'open-run', run: EvaluationRun): void
  (event: 'compare', payload: { baselineRunId: string; candidateRunId: string }): void
}>()

const baselineRunId = ref('')
const candidateRunId = ref('')
const drawerVisible = ref(false)
const selectedResult = ref<EvaluationRunResult | null>(null)

const completedRuns = computed(() => props.runs.filter(run => run.status === 'completed'))

const runColumns = [
  { colKey: 'status', title: '状态', width: 100, cell: 'status' },
  { colKey: 'progress', title: '进度', width: 220, cell: 'progress' },
  { colKey: 'metrics', title: '聚合指标', width: 100, cell: 'metrics' },
  { colKey: 'created_at', title: '创建时间', width: 190 },
  { colKey: 'operation', title: '操作', width: 90, cell: 'operation' },
]
const resultColumns = [
  { colKey: 'sample_index', title: '#', width: 60 },
  { colKey: 'question', title: '问题', ellipsis: true },
  { colKey: 'answer', title: '生成答案 / 错误', cell: 'answer', ellipsis: true },
  { colKey: 'status', title: '状态', width: 100, cell: 'status' },
  { colKey: 'metrics', title: '已评分', width: 80, cell: 'metrics' },
  { colKey: 'duration_ms', title: '耗时(ms)', width: 100 },
]
const comparisonColumns = [
  { colKey: 'metric', title: '指标', cell: 'metric' },
  { colKey: 'score', title: '基线 → 候选', cell: 'score' },
  { colKey: 'delta', title: '绝对差值', cell: 'delta' },
  { colKey: 'comparable_sample_count', title: '可比较样本' },
  { colKey: 'improved', title: '结论', cell: 'improved' },
]

function openRun(payload: { row: EvaluationRun }) {
  drawerVisible.value = true
  emit('open-run', payload.row)
}

function handleCompare() {
  if (!baselineRunId.value || !candidateRunId.value) {
    MessagePlugin.warning('请选择基线运行和候选运行')
    return
  }
  if (baselineRunId.value === candidateRunId.value) {
    MessagePlugin.warning('请选择两个不同的运行')
    return
  }
  emit('compare', {
    baselineRunId: baselineRunId.value,
    candidateRunId: candidateRunId.value,
  })
}

function selectResult(payload: { row: EvaluationRunResult }) {
  selectedResult.value = payload.row
}

function statusText(status: EvaluationRun['status']) {
  return ({ pending: '等待中', running: '运行中', completed: '已完成', failed: '失败' } as const)[status]
}

function statusTheme(status: EvaluationRun['status']) {
  return status === 'completed' ? 'success' : status === 'failed' ? 'danger' : status === 'running' ? 'primary' : 'default'
}

function progress(run: EvaluationRun) {
  return run.total_samples ? Math.round(run.finished_samples * 100 / run.total_samples) : 0
}

function formatScore(value: number) {
  return Number(value).toFixed(4)
}

function signedScore(value: number) {
  return `${value > 0 ? '+' : ''}${formatScore(value)}`
}

function metricScoreList(scores: Record<string, MetricScore>) {
  return Object.values(scores || {})
}

function runLabel(run: EvaluationRun) {
  return `${run.created_at} · ${run.id.slice(0, 8)}`
}

watch(() => props.selectedRun, run => {
  drawerVisible.value = !!run
  selectedResult.value = null
})

watch(drawerVisible, visible => {
  if (!visible) selectedResult.value = null
})
</script>

<style scoped lang="less">
.history-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 18px;
  margin-bottom: 24px;
}

.breadcrumb {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-family: var(--app-font-family);
  font-size: 20px;
  font-weight: 600;
  line-height: 30px;
}

.breadcrumb-link {
  border: none;
  background: transparent;
  padding: 4px 8px;
  margin: -4px -8px;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  font: inherit;
  border-radius: 6px;

  &:hover {
    color: var(--td-success-color);
    background: var(--td-bg-color-container-hover);
  }
}

.current-link {
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.breadcrumb-separator {
  color: var(--td-text-color-placeholder);
}

.breadcrumb-current {
  color: var(--td-text-color-primary);
}

.history-subtitle {
  margin: 6px 0 0;
  color: var(--td-text-color-placeholder);
  font-size: 14px;
}

.history-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.history-panel {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  padding: 16px;
  background: var(--td-bg-color-container);
}

.compare-controls {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.compare-arrow,
.metric-version,
.muted {
  color: var(--td-text-color-placeholder);
}

.comparison-panel {
  margin-top: 22px;

  h2 {
    margin: 0 0 12px;
    font-size: 16px;
  }
}

.improved {
  color: var(--td-success-color);
  font-weight: 600;
}

.declined {
  color: var(--td-error-color);
}

.run-summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 18px;
}

.run-summary > div {
  padding: 14px;
  border-radius: 8px;
  background: var(--td-bg-color-secondarycontainer);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.run-summary span {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.run-summary code {
  overflow: hidden;
  text-overflow: ellipsis;
}

.snapshot {
  margin: 16px 0;
  padding: 12px 14px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
}

.snapshot summary {
  cursor: pointer;
  font-weight: 600;
}

.snapshot pre {
  max-height: 320px;
  overflow: auto;
  margin: 12px 0 0;
  padding: 12px;
  border-radius: 6px;
  background: var(--td-bg-color-secondarycontainer);
  font-size: 12px;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 12px;
}

.metric-card {
  padding: 14px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.metric-card strong {
  font-size: 22px;
}

.metric-card span,
.metric-card small {
  color: var(--td-text-color-secondary);
  font-size: 12px;
}

.result-detail {
  margin-top: 18px;
  padding-top: 8px;
  border-top: 1px solid var(--td-component-stroke);
}

.evidence-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 24px;
}

.evidence-grid p {
  white-space: pre-wrap;
  line-height: 1.65;
}

.evidence-grid ol {
  padding-left: 22px;
  max-height: 480px;
  overflow: auto;
}

.evidence-grid li {
  margin-bottom: 14px;
  border-bottom: 1px solid var(--td-component-stroke);
}

.clamp {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 900px) {
  .history-header {
    flex-direction: column;
  }

  .compare-controls {
    grid-template-columns: 1fr;
  }

  .compare-arrow {
    display: none;
  }

  .run-summary,
  .evidence-grid {
    grid-template-columns: 1fr;
  }
}
</style>
