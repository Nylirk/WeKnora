<template>
  <t-dialog
    v-model:visible="dialogVisible"
    :header="$t('questionBank.importTitle')"
    :width="720"
    :confirm-btn="{ content: $t('questionBank.importConfirm'), loading: importing }"
    :cancel-btn="{ content: $t('common.cancel'), disabled: importing }"
    @confirm="doImport"
    @close="dialogVisible = false"
  >
    <div class="import-mode">
      <span class="import-mode-label">{{ $t('questionBank.importMethod') }}</span>
      <t-radio-group v-model="importMode" variant="default-filled">
        <t-radio-button value="paste">{{ $t('questionBank.pasteMode') }}</t-radio-button>
        <t-radio-button value="file">{{ $t('questionBank.fileMode') }}</t-radio-button>
      </t-radio-group>
    </div>

    <p class="format-help">{{ $t('questionBank.formatHelp') }}</p>

    <t-form v-if="importMode === 'paste'" label-align="top">
      <t-form-item :label="$t('questionBank.pasteJsonl')">
        <t-textarea
          v-model="pastedData"
          :autosize="{ minRows: 8, maxRows: 20 }"
          :placeholder="$t('questionBank.jsonPlaceholder')"
        />
      </t-form-item>
    </t-form>

    <div
      v-else
      class="file-upload-area"
      @dragover.prevent
      @drop.prevent="onFileDrop"
    >
      <input
        ref="fileInput"
        class="file-input"
        type="file"
        accept=".json,.jsonl,application/json,text/plain"
        @change="onFileSelected"
      >
      <t-icon name="upload" size="28px" />
      <span>{{ selectedFileName || $t('questionBank.fileUploadHint') }}</span>
      <t-button size="small" variant="outline" @click="fileInput?.click()">
        {{ $t('questionBank.selectFile') }}
      </t-button>
    </div>

    <div v-if="hasInput" class="import-preview">
      <t-space size="small" break-line>
        <t-tag variant="light">{{ $t('questionBank.parsedCount', { count: parsed.items.length }) }}</t-tag>
        <t-tag theme="warning" variant="light">{{ $t('questionBank.duplicateCount', { count: classified.duplicateItems.length }) }}</t-tag>
        <t-tag theme="success" variant="light">{{ $t('questionBank.importCount', { count: itemsToImport.length }) }}</t-tag>
        <t-tag :theme="parseErrors.length ? 'danger' : 'default'" variant="light">
          {{ $t('questionBank.parseErrorCount', { count: parseErrors.length }) }}
        </t-tag>
      </t-space>
      <div class="duplicate-toggle">
        <span>{{ $t('questionBank.allowDuplicates') }}</span>
        <t-switch v-model="allowDuplicates" size="small" />
      </div>
      <t-alert v-if="classified.duplicateItems.length" theme="warning" :close-btn="false">
        {{ $t('questionBank.duplicateHelp') }}
      </t-alert>
    </div>

    <div v-if="parseErrors.length" class="import-errors">
      <p class="error-title">{{ $t('questionBank.parseErrors') }}</p>
      <t-list size="small">
        <t-list-item v-for="(error, index) in parseErrors" :key="index">
          <span class="error-line">第 {{ error.line_number }} 行：{{ error.message }}</span>
        </t-list-item>
      </t-list>
    </div>
    <div v-if="importWarnings.length" class="import-warnings">
      <p class="warning-title">{{ $t('questionBank.importWarnings') }}</p>
      <t-list size="small">
        <t-list-item v-for="(warning, index) in importWarnings" :key="index">
          <span class="warning-line">第 {{ warning.line_number }} 行：{{ warning.message }}</span>
        </t-list-item>
      </t-list>
    </div>

    <div v-if="processingStatus" class="import-processing-status">
      <t-divider />
      <h4>后台处理状态</h4>
      <t-descriptions bordered size="small" :column="1">
        <t-descriptions-item label="处理阶段">
          <t-tag :theme="processingStatus.stage === 'failed' ? 'danger' : processingStatus.stage === 'ready_for_review' ? 'success' : 'primary'" variant="light" size="small">
            {{ stageLabel(processingStatus.stage) }}
          </t-tag>
        </t-descriptions-item>
        <t-descriptions-item v-if="processingStatus.auto_tagging_enabled" label="自动知识点关联">已启用</t-descriptions-item>
        <t-descriptions-item v-else label="自动知识点关联">
          <span class="skipped-text">{{ processingStatus.skipped_auto_tagging_reason || '已禁用' }}</span>
        </t-descriptions-item>
        <t-descriptions-item v-if="processingStatus.syllabus_check_enabled" label="自动考纲筛选">已启用</t-descriptions-item>
        <t-descriptions-item v-else label="自动考纲筛选">
          <span class="skipped-text">{{ processingStatus.skipped_syllabus_reason || '已禁用' }}</span>
        </t-descriptions-item>
        <t-descriptions-item v-if="processingStatus.error_message" label="错误信息">
          <span class="error-text">{{ processingStatus.error_message }}</span>
        </t-descriptions-item>
        <t-descriptions-item v-if="processingStatus.stage === 'ready_for_review'" label="操作建议">
          题目已完成后台处理，请进入题库编辑页审核题目、知识点、考纲适用性、难度和考试频率。
        </t-descriptions-item>
      </t-descriptions>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { importQuestions, getQuestionSetProcessingStatus, type ImportQuestionError, type Question, type QuestionSetProcessingStatus, type QuestionSetProcessingStage } from '@/api/question'
import {
  classifyQuestionImportItems,
  parseQuestionImportInput,
  selectQuestionImportItems,
} from '../questionData'

const props = withDefaults(defineProps<{
  visible: boolean
  setId: string
  knowledgeBaseId: string
  currentQuestions?: Question[]
}>(), {
  currentQuestions: () => [],
})
const emit = defineEmits<{ 'update:visible': [value: boolean]; imported: [] }>()

const dialogVisible = computed({
  get: () => props.visible,
  set: (value: boolean) => emit('update:visible', value),
})
const importMode = ref<'paste' | 'file'>('paste')
const pastedData = ref('')
const fileData = ref('')
const selectedFileName = ref('')
const fileInput = ref<HTMLInputElement | null>(null)
const backendErrors = ref<ImportQuestionError[]>([])
const allowDuplicates = ref(false)
const importing = ref(false)
const processingStatus = ref<QuestionSetProcessingStatus | null>(null)
const pollingTimer = ref<ReturnType<typeof setInterval> | null>(null)
const activeRawData = computed(() => importMode.value === 'paste' ? pastedData.value : fileData.value)
const hasInput = computed(() => activeRawData.value.trim().length > 0)
const parsed = computed(() => parseQuestionImportInput(activeRawData.value))
const classified = computed(() => classifyQuestionImportItems(parsed.value.items, props.currentQuestions))
const itemsToImport = computed(() => selectQuestionImportItems(
  parsed.value.items,
  classified.value,
  allowDuplicates.value,
))
const parseErrors = computed(() => [...parsed.value.errors, ...backendErrors.value])
const importWarnings = computed(() => parsed.value.warnings)

async function readFile(file: File) {
  if (!/\.(json|jsonl)$/i.test(file.name)) {
    selectedFileName.value = ''
    fileData.value = ''
    MessagePlugin.warning('仅支持 .json 或 .jsonl 文件')
    return
  }
  try {
    fileData.value = await file.text()
    selectedFileName.value = file.name
    backendErrors.value = []
  } catch (e: any) {
    selectedFileName.value = ''
    fileData.value = ''
    MessagePlugin.error(e?.message || '读取文件失败')
  }
}

async function onFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) await readFile(file)
  input.value = ''
}

async function onFileDrop(event: DragEvent) {
  const file = event.dataTransfer?.files?.[0]
  if (file) await readFile(file)
}

function stageLabel(stage: QuestionSetProcessingStage): string {
  const map: Record<string, string> = {
    '': '未开始',
    draft_imported: '已导入为草稿',
    indexing: '向量化中',
    auto_tagging: '知识点匹配中',
    syllabus_checking: '考纲筛选中',
    ready_for_review: '待人工审核',
    failed: '处理失败',
  }
  return map[stage] || stage
}

async function pollProcessingStatus() {
  try {
    const response: any = await getQuestionSetProcessingStatus(props.knowledgeBaseId, props.setId)
    processingStatus.value = response?.data ?? response
    if (processingStatus.value) {
      const stage = processingStatus.value.stage
      if (stage === 'ready_for_review' || stage === 'failed' || stage === '') {
        stopPolling()
        if (stage === 'ready_for_review') {
          MessagePlugin.success('题目已导入为草稿，后台处理完成，可进入题库编辑页审核')
        } else if (stage === 'failed') {
          MessagePlugin.error(`后台处理失败：${processingStatus.value.error_message || '未知错误'}`)
        }
      }
    }
  } catch {
    // polling is best-effort
  }
}

function startPolling() {
  stopPolling()
  pollProcessingStatus()
  pollingTimer.value = setInterval(pollProcessingStatus, 3000)
}

function stopPolling() {
  if (pollingTimer.value !== null) {
    clearInterval(pollingTimer.value)
    pollingTimer.value = null
  }
}

async function doImport() {
  backendErrors.value = []
  processingStatus.value = null
  if (parsed.value.items.length === 0) {
    MessagePlugin.warning('没有可导入的题目')
    return
  }
  const items = itemsToImport.value
  if (items.length === 0 && classified.value.duplicateItems.length > 0) {
    MessagePlugin.warning('没有可导入的新题，已跳过重复题。')
    return
  }
  importing.value = true
  try {
    const response: any = await importQuestions(props.knowledgeBaseId, props.setId, { items })
    const result = response?.data ?? response
    backendErrors.value = Array.isArray(result?.errors) ? result.errors : []
    const created = result?.created || 0
    if (created > 0) {
      MessagePlugin.success(`已导入 ${created} 道题目为草稿，系统正在进行自动处理。处理完成后请人工确认。`)
      const missingAnswerCount = items.filter(item => !item.answer_text.trim()).length
      if (missingAnswerCount) {
        MessagePlugin.warning(`${missingAnswerCount} 道题缺少答案，审核或导出前需要补全`)
      }
      emit('imported')
      // Start polling for processing status
      startPolling()
    } else {
      MessagePlugin.warning('没有题目被导入')
    }
    if (parseErrors.value.length === 0 && created === 0) dialogVisible.value = false
  } catch (e: any) {
    MessagePlugin.error(e?.message || '导入失败')
  } finally {
    importing.value = false
  }
}

watch(() => props.visible, visible => {
  if (!visible) {
    stopPolling()
    return
  }
  importMode.value = 'paste'
  pastedData.value = ''
  fileData.value = ''
  selectedFileName.value = ''
  backendErrors.value = []
  processingStatus.value = null
  allowDuplicates.value = false
})
</script>

<style scoped>
.import-mode { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 8px; }
.import-mode-label { font-weight: 500; }
.format-help { margin: 0 0 14px; color: var(--td-text-color-secondary); font-size: 12px; }
.file-upload-area { min-height: 180px; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 10px; margin-bottom: 16px; border: 1px dashed var(--td-component-stroke); border-radius: 6px; color: var(--td-text-color-secondary); background: var(--td-bg-color-secondarycontainer); }
.file-input { display: none; }
.import-preview { display: flex; flex-direction: column; gap: 10px; margin-bottom: 12px; }
.duplicate-toggle { display: flex; align-items: center; gap: 8px; font-size: 13px; }
.import-errors,
.import-warnings { margin-top: 12px; }
.error-title,
.warning-title { margin: 0 0 8px; font-weight: 600; }
.error-title,
.error-line { color: var(--td-error-color); }
.warning-title,
.warning-line { color: var(--td-warning-color); }
.import-processing-status { margin-top: 12px; }
.import-processing-status h4 { margin: 0 0 8px; font-size: 14px; }
.skipped-text { color: var(--td-text-color-placeholder); }
.error-text { color: var(--td-error-color); }
</style>
