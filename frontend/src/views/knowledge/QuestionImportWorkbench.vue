<template>
  <t-dialog
    :visible="visible"
    :header="false"
    :footer="false"
    :close-btn="false"
    width="90vw"
    top="5vh"
    :z-index="3500"
    dialog-class-name="question-import-workbench-dialog"
    :close-on-overlay-click="false"
    :close-on-esc-keydown="false"
  >
    <div class="workbench-shell">
      <div class="workbench-header">
        <div class="workbench-heading">
          <h3>题库导入工作台</h3>
          <t-steps :current="store.currentStep === 'block-review' ? 0 : 1" size="small" class="header-steps">
            <t-step-item title="Block Review" />
            <t-step-item title="Question Review" />
          </t-steps>
        </div>
        <t-space size="small">
          <t-button variant="outline" @click="handleAbandon">放弃导入</t-button>
          <t-button v-if="store.currentStep === 'block-review'" theme="primary" @click="goToQuestionReview">
            下一步：题目解析
          </t-button>
          <t-button v-else variant="outline" @click="returnToBlockReview">
            返回 Block Review
          </t-button>
        </t-space>
      </div>

      <div class="workbench-configbar">
        <div class="config-control">
          <span class="config-label">默认难度</span>
          <t-select v-model="store.defaultDifficulty" size="small" style="width: 104px" @change="saveDebounced">
            <t-option value="easy" label="简单" />
            <t-option value="medium" label="中等" />
            <t-option value="hard" label="困难" />
          </t-select>
        </div>
        <span class="config-divider" />
        <div class="config-meta"><span>当前格式</span><strong>{{ importFormatLabel }}</strong></div>
        <div class="config-meta"><span>Preset</span><strong>{{ store.strategyPreset }}</strong></div>
        <span class="config-divider" />
        <t-tag variant="light">{{ store.summary.total_blocks }} blocks</t-tag>
        <t-tag v-if="anomalyCounts.error > 0" theme="danger" variant="light">{{ anomalyCounts.error }} errors</t-tag>
        <t-tag v-if="anomalyCounts.warning > 0" theme="warning" variant="light">{{ anomalyCounts.warning }} warnings</t-tag>
        <t-tag v-if="anomalyCounts.error === 0 && anomalyCounts.warning === 0" theme="success" variant="light">无异常</t-tag>
      </div>

      <div class="workbench-body">
        <BlockReviewPanel v-if="store.currentStep === 'block-review'" @changed="saveDebounced" />
        <QuestionReviewPanel
          v-else
          ref="questionReviewRef"
          @changed="saveDebounced"
          @imported="handleImported"
        />
      </div>
    </div>
  </t-dialog>

  <t-dialog
    v-model:visible="abandonVisible"
    header="放弃导入"
    attach="body"
    :z-index="4500"
    :close-btn="false"
    :close-on-overlay-click="false"
    :confirm-btn="{ content: '保存草稿', theme: 'primary' }"
    :cancel-btn="{ content: '直接放弃' }"
    @confirm="abandonSaveDraft"
    @cancel="abandonDiscard"
  >
    <p>是否保存当前草稿？草稿将保留 7 天。</p>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useImportWorkbenchStore } from '@/stores/importWorkbench'
import { deleteDraft, saveDraft } from '@/utils/importDraftDB'
import BlockReviewPanel from './components/BlockReviewPanel.vue'
import QuestionReviewPanel from './components/QuestionReviewPanel.vue'

const props = defineProps<{
  visible: boolean
  kbId: string
  setId: string
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  imported: []
  abandoned: []
}>()

const store = useImportWorkbenchStore()
const questionReviewRef = ref<InstanceType<typeof QuestionReviewPanel> | null>(null)
const abandonVisible = ref(false)

const importFormatLabel = computed(() => {
  if (store.importFormat === 'pdf') return 'PDF'
  if (store.importFormat === 'word') return 'Word / DOCX'
  return 'JSON / JSONL'
})

const anomalyCounts = computed(() => {
  let error = 0
  let warning = 0
  for (const block of store.blocks) {
    const anomalies = Array.isArray(block.anomalies) ? block.anomalies : []
    for (const anomaly of anomalies) {
      if (anomaly?.severity === 'error') error += 1
      if (anomaly?.severity === 'warning') warning += 1
    }
  }
  return { error, warning }
})

let saveTimer: ReturnType<typeof setTimeout> | null = null

function clearSaveTimer() {
  if (!saveTimer) return
  clearTimeout(saveTimer)
  saveTimer = null
}

function saveDebounced() {
  clearSaveTimer()
  saveTimer = setTimeout(() => {
    saveTimer = null
    void saveProgress().catch(() => {})
  }, 800)
}

async function saveProgress() {
  clearSaveTimer()
  if (!store.kbId || !store.setId || store.blocks.length === 0) return
  await saveDraft({
    kbId: store.kbId,
    setId: store.setId,
    blocks: store.blocks,
    strategyPreset: store.strategyPreset,
    defaultDifficulty: store.defaultDifficulty,
    importMode: store.importMode,
    importFormat: store.importFormat,
    currentStep: store.currentStep,
    questions: store.questions,
    timestamp: Date.now(),
  })
}

onBeforeUnmount(() => {
  if (saveTimer) void saveProgress().catch(() => {})
})

async function goToQuestionReview() {
  store.goToStep('question-review')
  await saveProgress()
  await nextTick()
  await questionReviewRef.value?.parseQuestions()
}

function returnToBlockReview() {
  store.goToStep('block-review')
  saveDebounced()
}

function handleAbandon() {
  abandonVisible.value = true
}

async function abandonSaveDraft() {
  await saveProgress()
  MessagePlugin.success('草稿已保存（7 天有效）')
  abandonVisible.value = false
  store.reset()
  emit('update:visible', false)
  emit('abandoned')
}

async function abandonDiscard() {
  await deleteDraft(props.kbId, props.setId)
  clearSaveTimer()
  store.reset()
  abandonVisible.value = false
  emit('update:visible', false)
  emit('abandoned')
}

function handleImported() {
  clearSaveTimer()
  emit('update:visible', false)
  emit('imported')
}
</script>

<style scoped>
.workbench-shell { display: flex; flex-direction: column; height: 100%; min-height: 0; }
.workbench-header { display: flex; align-items: center; justify-content: space-between; gap: 20px; padding: 14px 20px; border-bottom: 1px solid var(--td-component-stroke); background: var(--td-bg-color-container); }
.workbench-heading { min-width: 0; display: flex; align-items: center; gap: 28px; }
.workbench-heading h3 { flex-shrink: 0; margin: 0; font-size: 17px; font-weight: 600; }
.header-steps { width: 300px; }
.workbench-configbar { min-height: 44px; display: flex; align-items: center; gap: 12px; padding: 7px 20px; border-bottom: 1px solid var(--td-component-stroke); background: var(--td-bg-color-page); }
.config-control { display: flex; align-items: center; gap: 7px; }
.config-label, .config-meta span { font-size: 12px; color: var(--td-text-color-secondary); }
.config-meta { display: flex; align-items: center; gap: 5px; font-size: 12px; }
.config-meta strong { font-weight: 600; color: var(--td-text-color-primary); }
.config-divider { width: 1px; height: 20px; background: var(--td-component-stroke); }
.workbench-body { flex: 1; min-height: 0; overflow: hidden; padding: 0 20px 14px; }
</style>

<style>
.question-import-workbench-dialog { height: 90vh; max-height: 90vh; display: flex; flex-direction: column; padding: 0; overflow: hidden; }
.question-import-workbench-dialog .t-dialog__body { flex: 1; min-height: 0; padding: 0; overflow: hidden; }
</style>
