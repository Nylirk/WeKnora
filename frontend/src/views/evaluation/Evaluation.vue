<template>
  <div class="evaluation-page">
    <template v-if="!datasetId">
      <div class="evaluation-header">
        <div>
          <h1>评测集</h1>
          <p>管理固定问答样本，用于评估知识库检索与回答效果。</p>
        </div>
        <t-button v-if="canEdit" theme="primary" @click="openDatasetDialog()">
          <template #icon><t-icon name="add" /></template>
          创建评测集
        </t-button>
      </div>

      <EvaluationDatasetSection
        :datasets="datasets"
        :loading="loadingDatasets"
        :can-edit="canEdit"
        :collapsed="datasetSectionCollapsed"
        @toggle="datasetSectionCollapsed = !datasetSectionCollapsed"
        @open="openDataset"
        @edit="openDatasetDialog"
        @delete="removeDataset"
      />
    </template>

    <template v-else-if="selectedDataset">
      <EvaluationRunHistory
        v-if="isHistoryView"
        :dataset="selectedDataset"
        :runs="datasetRuns"
        :run-results="runResults"
        :selected-run="selectedRun"
        :comparison="comparison"
        :loading="loadingRuns"
        :loading-run-results="loadingRunResults"
        :comparing="comparing"
        @back="goList"
        @samples="goDatasetDetail"
        @create-run="openRunDialog"
        @open-run="openRunDetail"
        @compare="compareRuns"
      />
      <EvaluationDatasetDetail
        v-else
        :dataset="selectedDataset"
        :samples="samples"
        :loading="loadingSamples"
        :can-edit="canEdit"
        @back="goList"
        @history="goHistory"
        @create-run="openRunDialog"
        @add-sample="openSampleDialog()"
        @edit-sample="openSampleDialog"
        @delete-sample="removeSample"
        @import-samples="sampleImportVisible = true"
      />
    </template>

    <div v-else class="dataset-missing">
      <t-empty description="评测集不存在或已被删除" />
      <t-button variant="outline" @click="goList">返回评测集</t-button>
    </div>

    <t-dialog
      v-model:visible="datasetDialogVisible"
      :header="editingDataset ? '编辑评测集' : '创建评测集'"
      :confirm-btn="{ content: '保存', loading: saving }"
      @confirm="saveDataset"
    >
      <t-form label-align="top">
        <t-form-item label="名称" required>
          <t-input v-model="datasetForm.name" maxlength="255" />
        </t-form-item>
        <t-form-item label="描述">
          <t-textarea v-model="datasetForm.description" :autosize="{ minRows: 3, maxRows: 6 }" />
        </t-form-item>
      </t-form>
    </t-dialog>

    <EvaluationSampleDialog
      v-model:visible="sampleDialogVisible"
      :sample="editingSample"
      :saving="saving"
      @save="saveSample"
    />

    <EvaluationSampleImportDialog
      v-model:visible="sampleImportVisible"
      :saving="saving"
      @import="importSamples"
    />

    <EvaluationRunDialog
      v-model:visible="runDialogVisible"
      :dataset="selectedDataset"
      :metrics="metrics"
      :knowledge-bases="knowledgeBases"
      :models="models"
      :saving="saving"
      @save="saveRun"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { useAuthStore } from '@/stores/auth'
import { listKnowledgeBases } from '@/api/knowledge-base'
import { listModels, type ModelConfig } from '@/api/model'
import {
  compareEvaluationRuns,
  createEvaluationDataset,
  createEvaluationRun,
  createEvaluationSample,
  deleteEvaluationDataset,
  deleteEvaluationSample,
  getEvaluationRun,
  listEvaluationDatasets,
  listEvaluationMetrics,
  listEvaluationRunResults,
  listEvaluationRuns,
  listEvaluationSamples,
  updateEvaluationDataset,
  updateEvaluationSample,
  type EvaluationDataset,
  type EvaluationRun,
  type EvaluationRunResult,
  type EvaluationSample,
  type MetricDefinition,
  type ReferenceContext,
  type RunComparison,
} from '@/api/evaluation'
import EvaluationDatasetSection from './components/EvaluationDatasetSection.vue'
import EvaluationDatasetDetail from './components/EvaluationDatasetDetail.vue'
import EvaluationSampleDialog from './components/EvaluationSampleDialog.vue'
import EvaluationSampleImportDialog from './components/EvaluationSampleImportDialog.vue'
import EvaluationRunDialog from './components/EvaluationRunDialog.vue'
import EvaluationRunHistory from './components/EvaluationRunHistory.vue'

type RunPayload = {
  dataset_id: string
  knowledge_base_id: string
  chat_model_id: string
  rerank_model_id?: string
  vector_threshold: number
  keyword_threshold: number
  embedding_top_k: number
  rerank_top_k: number
  rerank_threshold: number
  metric_names: string[]
}

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const canEdit = computed(() => authStore.hasRole('admin'))

const datasets = ref<EvaluationDataset[]>([])
const samples = ref<EvaluationSample[]>([])
const runs = ref<EvaluationRun[]>([])
const metrics = ref<MetricDefinition[]>([])
const knowledgeBases = ref<any[]>([])
const models = ref<ModelConfig[]>([])
const selectedRun = ref<EvaluationRun | null>(null)
const runResults = ref<EvaluationRunResult[]>([])
const comparison = ref<RunComparison | null>(null)

const loadingDatasets = ref(false)
const loadingSamples = ref(false)
const loadingRuns = ref(false)
const loadingRunResults = ref(false)
const saving = ref(false)
const comparing = ref(false)
const datasetSectionCollapsed = ref(false)
const datasetDialogVisible = ref(false)
const sampleDialogVisible = ref(false)
const sampleImportVisible = ref(false)
const runDialogVisible = ref(false)
const editingDataset = ref<EvaluationDataset | null>(null)
const editingSample = ref<EvaluationSample | null>(null)
const datasetForm = reactive({ name: '', description: '' })

const datasetId = computed(() => typeof route.query.datasetId === 'string' ? route.query.datasetId : '')
const isHistoryView = computed(() => route.query.view === 'history')
const selectedDataset = computed(() => datasets.value.find(dataset => dataset.id === datasetId.value) || null)
const datasetRuns = computed(() => runs.value.filter(run => run.dataset_id === datasetId.value))

function routeQuery(query: Record<string, string | undefined>) {
  return router.push({ path: '/evaluation', query })
}

function openDataset(dataset: EvaluationDataset) {
  void routeQuery({ datasetId: dataset.id })
}

function goList() {
  selectedRun.value = null
  runResults.value = []
  comparison.value = null
  void routeQuery({})
}

function goDatasetDetail() {
  if (!datasetId.value) return
  comparison.value = null
  void routeQuery({ datasetId: datasetId.value })
}

function goHistory() {
  if (!datasetId.value) return
  void routeQuery({ datasetId: datasetId.value, view: 'history' })
}

async function loadDatasets() {
  loadingDatasets.value = true
  try {
    datasets.value = (await listEvaluationDatasets()).data
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载评测集失败')
  } finally {
    loadingDatasets.value = false
  }
}

async function loadSamples() {
  if (!datasetId.value) {
    samples.value = []
    return
  }
  loadingSamples.value = true
  try {
    samples.value = (await listEvaluationSamples(datasetId.value)).data
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载样本失败')
  } finally {
    loadingSamples.value = false
  }
}

async function loadRuns() {
  loadingRuns.value = true
  try {
    runs.value = (await listEvaluationRuns()).data
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载历史记录失败')
  } finally {
    loadingRuns.value = false
  }
}

async function loadOptions() {
  try {
    const [metricList, kbResponse, modelList] = await Promise.all([
      listEvaluationMetrics(),
      listKnowledgeBases(),
      listModels(),
    ])
    metrics.value = metricList
    knowledgeBases.value = (kbResponse as any).data || []
    models.value = modelList
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载运行配置失败')
  }
}

function openDatasetDialog(row?: EvaluationDataset) {
  editingDataset.value = row || null
  datasetForm.name = row?.name || ''
  datasetForm.description = row?.description || ''
  datasetDialogVisible.value = true
}

async function saveDataset() {
  if (!datasetForm.name.trim()) {
    MessagePlugin.warning('请输入评测集名称')
    return
  }
  saving.value = true
  try {
    const payload = {
      name: datasetForm.name.trim(),
      description: datasetForm.description.trim(),
    }
    if (editingDataset.value) {
      await updateEvaluationDataset(editingDataset.value.id, payload)
    } else {
      await createEvaluationDataset(payload)
    }
    datasetDialogVisible.value = false
    await loadDatasets()
    MessagePlugin.success('评测集已保存')
  } catch (error: any) {
    MessagePlugin.error(error.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function removeDataset(row: EvaluationDataset) {
  try {
    await deleteEvaluationDataset(row.id)
    if (datasetId.value === row.id) {
      goList()
    }
    await loadDatasets()
    MessagePlugin.success('评测集已删除')
  } catch (error: any) {
    MessagePlugin.error(error.message || '删除失败')
  }
}

function openSampleDialog(row?: EvaluationSample) {
  if (!selectedDataset.value) return
  editingSample.value = row || null
  sampleDialogVisible.value = true
}

async function saveSample(payload: {
  question: string
  reference_answer: string
  reference_contexts: ReferenceContext[]
}) {
  if (!selectedDataset.value) return
  saving.value = true
  try {
    if (editingSample.value) {
      await updateEvaluationSample(selectedDataset.value.id, editingSample.value.id, payload)
    } else {
      await createEvaluationSample(selectedDataset.value.id, payload)
    }
    sampleDialogVisible.value = false
    await Promise.all([loadSamples(), loadDatasets()])
    MessagePlugin.success('样本已保存')
  } catch (error: any) {
    MessagePlugin.error(error.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function removeSample(row: EvaluationSample) {
  if (!selectedDataset.value) return
  try {
    await deleteEvaluationSample(selectedDataset.value.id, row.id)
    await Promise.all([loadSamples(), loadDatasets()])
    MessagePlugin.success('样本已删除')
  } catch (error: any) {
    MessagePlugin.error(error.message || '删除失败')
  }
}

async function importSamples(items: Array<{
  question: string
  reference_answer: string
  reference_contexts: ReferenceContext[]
}>) {
  if (!selectedDataset.value) return
  saving.value = true
  try {
    for (const item of items) {
      await createEvaluationSample(selectedDataset.value.id, item)
    }
    sampleImportVisible.value = false
    await Promise.all([loadSamples(), loadDatasets()])
    MessagePlugin.success(`已导入 ${items.length} 条样本`)
  } catch (error: any) {
    MessagePlugin.error(error.message || '导入失败')
  } finally {
    saving.value = false
  }
}

function openRunDialog() {
  if (!selectedDataset.value) return
  runDialogVisible.value = true
}

async function saveRun(payload: RunPayload) {
  saving.value = true
  try {
    await createEvaluationRun({
      ...payload,
      metrics: payload.metric_names.map(name => ({
        name,
        version: metrics.value.find(metric => metric.name === name)?.version || 'v1',
      })),
      metric_names: undefined,
    })
    runDialogVisible.value = false
    await loadRuns()
    MessagePlugin.success('评测运行已创建')
  } catch (error: any) {
    MessagePlugin.error(error.message || '创建运行失败')
  } finally {
    saving.value = false
  }
}

async function openRunDetail(run: EvaluationRun) {
  selectedRun.value = run
  loadingRunResults.value = true
  try {
    runResults.value = (await listEvaluationRunResults(run.id)).data
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载运行详情失败')
  } finally {
    loadingRunResults.value = false
  }
}

async function compareRuns(payload: { baselineRunId: string; candidateRunId: string }) {
  comparing.value = true
  try {
    comparison.value = await compareEvaluationRuns(payload.baselineRunId, payload.candidateRunId)
  } catch (error: any) {
    MessagePlugin.error(error.message || '对比失败')
  } finally {
    comparing.value = false
  }
}

let pollTimer: number | undefined

watch(datasetId, () => {
  selectedRun.value = null
  runResults.value = []
  comparison.value = null
  void loadSamples()
}, { immediate: true })

onMounted(async () => {
  await Promise.all([loadDatasets(), loadRuns(), loadOptions()])
  pollTimer = window.setInterval(async () => {
    if (runs.value.some(run => run.status === 'pending' || run.status === 'running')) {
      await loadRuns()
      if (selectedRun.value && (selectedRun.value.status === 'pending' || selectedRun.value.status === 'running')) {
        selectedRun.value = await getEvaluationRun(selectedRun.value.id)
        await openRunDetail(selectedRun.value)
      }
    }
  }, 2500)
})

onBeforeUnmount(() => {
  if (pollTimer) window.clearInterval(pollTimer)
})
</script>

<style scoped lang="less">
.evaluation-page {
  min-height: 100%;
  height: 100%;
  overflow: auto;
  padding: 20px 28px 28px;
  box-sizing: border-box;
  background: var(--td-bg-color-container);
  color: var(--td-text-color-primary);
}

.evaluation-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 22px;

  h1 {
    margin: 0;
    color: var(--td-text-color-primary);
    font-family: var(--app-font-family);
    font-size: 24px;
    font-weight: 600;
    line-height: 32px;
  }

  p {
    margin: 6px 0 0;
    color: var(--td-text-color-placeholder);
    font-size: 14px;
    line-height: 20px;
  }
}

.dataset-missing {
  min-height: 360px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
}

@media (max-width: 720px) {
  .evaluation-page {
    padding: 16px;
  }

  .evaluation-header {
    flex-direction: column;
  }
}
</style>
