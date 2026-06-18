<template>
  <t-dialog
    v-model:visible="dialogVisible"
    :header="dialogTitle"
    :width="800"
    :confirm-btn="null"
    :cancel-btn="{ content: $t('common.cancel') }"
    :close-on-overlay-click="false"
    @close="closeAndReset"
  >
    <!-- 1. File upload area -->
    <div class="file-upload-area">
      <label class="file-upload-label">
        <input
          ref="fileInputRef"
          type="file"
          :accept="accept"
          class="file-input"
          @change="onFileSelected"
        />
        <div class="file-upload-body">
          <t-icon name="upload" size="28px" />
          <span v-if="selectedFile">{{ $t('questionBank.fileImportSelected', { name: selectedFile.name, size: formatFileSize(selectedFile.size) }) }}</span>
          <span v-else>{{ $t('questionBank.fileImportSelect') }}</span>
          <t-button size="small" variant="outline" @click.stop="fileInputRef?.click()">
            {{ $t('questionBank.selectFile') }}
          </t-button>
        </div>
      </label>
    </div>

    <!-- 2. Parse config -->
    <div class="parse-config">
      <t-space size="small" break-line>
        <div class="config-item">
          <span class="config-label">{{ $t('questionBank.type', '题型') }}：</span>
          <t-select v-model="parseConfig.default_question_type" style="width: 120px" size="small">
            <t-option v-for="qt in questionTypes" :key="qt.value" :value="qt.value" :label="qt.label" />
          </t-select>
        </div>
        <div class="config-item">
          <span class="config-label">{{ $t('questionBank.difficulty', '难度') }}：</span>
          <t-select v-model="parseConfig.default_difficulty" style="width: 100px" size="small">
            <t-option value="easy" :label="$t('questionBank.easy', '简单')" />
            <t-option value="medium" :label="$t('questionBank.medium', '中等')" />
            <t-option value="hard" :label="$t('questionBank.hard', '困难')" />
          </t-select>
        </div>
        <t-button variant="outline" :loading="parsing" :disabled="!selectedFile" @click="doPreviewParse">
          {{ parsing ? $t('questionBank.fileImportParsing') : $t('questionBank.fileImportParsePreview') }}
        </t-button>
      </t-space>
    </div>

    <!-- 3. Preview area -->
    <div v-if="previewResult" class="preview-area">
      <!-- Stats -->
      <div class="preview-stats">
        <t-space size="small" break-line>
          <t-tag variant="light">{{ $t('questionBank.fileImportDetected') }}：{{ previewResult.stats?.detected_questions ?? questionItems.length }}</t-tag>
          <t-tag theme="success" variant="light">{{ $t('questionBank.fileImportWithAnswer') }}：{{ previewResult.stats?.with_answer ?? $t('questionBank.fileImportNoStat') }}</t-tag>
          <t-tag theme="warning" variant="light">{{ $t('questionBank.fileImportWithoutAnswer') }}：{{ previewResult.stats?.without_answer ?? $t('questionBank.fileImportNoStat') }}</t-tag>
          <t-tag v-if="previewResult.warnings.length" theme="danger" variant="light">
            {{ previewResult.warnings.length }} 条警告
          </t-tag>
        </t-space>
      </div>

      <!-- Warnings -->
      <t-alert v-if="previewResult.warnings.length" theme="warning" :close-btn="false">
        <t-list size="small">
          <t-list-item v-for="(w, i) in previewResult.warnings" :key="'warn-' + i">
            <span class="warning-text">{{ w }}</span>
          </t-list-item>
        </t-list>
      </t-alert>

      <!-- Errors -->
      <t-alert v-if="previewErrors.length" theme="error" :close-btn="false">
        <t-list size="small">
          <t-list-item v-for="(e, i) in previewErrors" :key="'err-' + i">
            <span class="error-text">{{ $t('questionBank.fileImportError', '错误') }}：{{ e.message }}</span>
          </t-list-item>
        </t-list>
      </t-alert>

      <!-- Raw text (collapsible) -->
      <t-collapse v-if="previewResult.raw_text_preview">
        <t-collapse-panel :header="$t('questionBank.fileImportRawText')">
          <pre class="raw-text">{{ previewResult.raw_text_preview }}</pre>
        </t-collapse-panel>
      </t-collapse>

      <!-- Question preview list -->
      <div v-if="questionItems.length" class="question-preview-list">
        <h4>{{ $t('questionBank.fileImportPreviewList') }}（{{ questionItems.length }}）</h4>
        <div v-for="(item, index) in questionItems" :key="index" class="question-preview-item">
          <div class="preview-item-header">
            <t-tag size="small">{{
              questionTypeLabel(item.question_type as QuestionType)
            }}</t-tag>
            <t-tag size="small" variant="light">{{ difficultyLabel(item.difficulty) }}</t-tag>
            <span v-if="!item.answer_text" class="no-answer-tag">{{ $t('questionBank.fileImportEmptyAnswer') }}</span>
            <t-space size="small">
              <t-button size="small" variant="text" @click="editPreviewItem(index)">
                {{ $t('questionBank.fileImportEdit') }}
              </t-button>
              <t-button size="small" variant="text" theme="danger" @click="removePreviewItem(index)">
                {{ $t('questionBank.fileImportDelete') }}
              </t-button>
            </t-space>
          </div>
          <div class="preview-item-stem">{{ item.stem_text }}</div>
          <div v-if="item.answer_text" class="preview-item-answer">
            <span class="answer-label">答案：</span>{{ item.answer_text }}
          </div>
          <div v-if="item.analysis_text" class="preview-item-analysis">
            <span class="analysis-label">解析：</span>{{ item.analysis_text }}
          </div>
        </div>
      </div>
      <t-empty v-else :description="$t('questionBank.fileImportNoQuestions')" />
    </div>

    <!-- 4. Import mode selector (shown after preview) -->
    <div v-if="previewResult && questionItems.length" class="import-mode-section">
      <div class="section-title">{{ $t('questionBank.fileImportImportMode') }}</div>
      <t-radio-group v-model="importMode" variant="default-filled">
        <t-radio-button value="draft">{{ $t('questionBank.fileImportModeDraft') }}</t-radio-button>
        <t-radio-button value="reviewed">{{ $t('questionBank.fileImportModeReviewed') }}</t-radio-button>
      </t-radio-group>
      <p class="import-mode-hint">{{ $t('questionBank.fileImportModeDraftHelp') }}</p>
    </div>

    <!-- Custom footer -->
    <template #footer>
      <t-space size="small">
        <t-button variant="outline" @click="closeAndReset">
          {{ $t('common.cancel', '取消') }}
        </t-button>
        <t-button
          theme="primary"
          :loading="importing"
          :disabled="!previewResult || !questionItems.length"
          @click="doConfirmImport"
        >
          {{ $t('questionBank.fileImportConfirm', '确认导入') }}
        </t-button>
      </t-space>
    </template>

    <!-- inline edit sub-dialog -->
    <t-dialog
      v-model:visible="editVisible"
      :header="$t('questionBank.editQuestion', '编辑题目')"
      width="600px"
      :confirm-btn="null"
    >
      <t-form v-if="editingItem" label-align="top">
        <t-form-item label="题型">
          <t-select v-model="editingItem.question_type" style="width: 100%">
            <t-option v-for="qt in questionTypes" :key="qt.value" :value="qt.value" :label="qt.label" />
          </t-select>
        </t-form-item>
        <t-form-item label="题干">
          <t-textarea v-model="editingItem.stem_text" :autosize="{ minRows: 2, maxRows: 6 }" />
        </t-form-item>
        <t-form-item label="答案">
          <t-textarea v-model="editingItem.answer_text" :autosize="{ minRows: 1, maxRows: 4 }" />
        </t-form-item>
        <t-form-item label="解析">
          <t-textarea v-model="editingItem.analysis_text" :autosize="{ minRows: 1, maxRows: 4 }" />
        </t-form-item>
        <t-form-item label="难度">
          <t-select v-model="editingItem.difficulty" style="width: 120px">
            <t-option value="easy" label="简单" />
            <t-option value="medium" label="中等" />
            <t-option value="hard" label="困难" />
          </t-select>
        </t-form-item>
      </t-form>
      <template #footer>
        <t-button variant="outline" @click="editVisible = false">{{ $t('common.cancel') }}</t-button>
        <t-button theme="primary" @click="saveEditedItem">{{ $t('common.save') }}</t-button>
      </template>
    </t-dialog>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { previewImportFile, importQuestions, type ImportFilePreviewResponse, type ImportQuestionItem, type QuestionType, type QuestionDifficulty } from '@/api/question'
import { classifyQuestionImportItems, selectQuestionImportItems } from '../questionData'

const { t } = useI18n()

const props = withDefaults(defineProps<{
  visible: boolean
  knowledgeBaseId: string
  setId: string
  importType: 'word' | 'pdf'
  currentQuestions?: any[]
}>(), {
  currentQuestions: () => [],
})

const emit = defineEmits<{ 'update:visible': [value: boolean]; imported: [] }>()

const dialogVisible = computed({
  get: () => props.visible,
  set: (value: boolean) => {
    if (!value) closeAndReset()
    // When value transitions to true from parent, that's handled by the watcher
  },
})

const accept = computed(() => {
  return props.importType === 'word' ? '.doc,.docx' : '.pdf'
})

const dialogTitle = computed(() => {
  return props.importType === 'word'
    ? (t('questionBank.fileImportWord') || 'Word / DOCX 导入题目')
    : (t('questionBank.fileImportPdf') || 'PDF 导入题目')
})

const questionTypes = [
  { value: 'single_choice', label: '单选' },
  { value: 'multiple_choice', label: '多选' },
  { value: 'true_false', label: '判断' },
  { value: 'fill_blank', label: '填空' },
  { value: 'short_answer', label: '简答' },
  { value: 'essay', label: '论述' },
  { value: 'composite', label: '复合' },
]

const fileInputRef = ref<HTMLInputElement | null>(null)
const selectedFile = ref<File | null>(null)
const parsing = ref(false)
const importing = ref(false)
const previewResult = ref<ImportFilePreviewResponse | null>(null)
const importMode = ref<'draft' | 'reviewed'>('draft')
const editVisible = ref(false)
const editingIndex = ref(-1)
const editingItem = ref<ImportQuestionItem | null>(null)

const parseConfig = ref({
  default_question_type: 'short_answer',
  default_difficulty: 'medium',
  mode: 'rule',
})

// --- Request cancellation ---
const previewAbortController = ref<AbortController | null>(null)
const activePreviewRequestId = ref(0)
const importingRequestId = ref(0)

function abortCurrentRequest() {
  if (previewAbortController.value) {
    previewAbortController.value.abort()
    previewAbortController.value = null
  }
  activePreviewRequestId.value++
}

function cleanupDialogState() {
  abortCurrentRequest()
  selectedFile.value = null
  previewResult.value = null
  parsing.value = false
  importing.value = false
  editVisible.value = false
  editingIndex.value = -1
  editingItem.value = null
  importMode.value = 'draft'
  parseConfig.value = {
    default_question_type: 'short_answer',
    default_difficulty: 'medium',
    mode: 'rule',
  }
  if (fileInputRef.value) {
    fileInputRef.value.value = ''
  }
  previewAbortController.value = null
}

let closeGuard = false
function closeAndReset() {
  if (closeGuard) return
  closeGuard = true
  cleanupDialogState()
  emit('update:visible', false)
  // Reset guard after microtask so next open can proceed cleanly
  Promise.resolve().then(() => { closeGuard = false })
}

const questionItems = computed(() => {
  return previewResult.value?.items ?? []
})

const previewErrors = computed(() => previewResult.value?.errors ?? [])

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

function questionTypeLabel(t2: QuestionType) {
  const map: Record<QuestionType, string> = {
    single_choice: '单选', multiple_choice: '多选', true_false: '判断',
    fill_blank: '填空', short_answer: '简答', essay: '论述', composite: '复合',
  }
  return map[t2] || t2
}

function difficultyLabel(d: string) {
  const map: Record<string, string> = { easy: '简单', medium: '中等', hard: '困难' }
  return map[d] || d
}

function onFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return

  // Abort any in-flight preview, clear old results
  abortCurrentRequest()
  previewResult.value = null
  parsing.value = false
  importing.value = false

  const ext = file.name.split('.').pop()?.toLowerCase()
  if (props.importType === 'word' && !['doc', 'docx'].includes(ext || '')) {
    MessagePlugin.warning('仅支持 DOC、DOCX 文件。')
    selectedFile.value = null
    input.value = ''
    return
  }
  if (props.importType === 'pdf' && ext !== 'pdf') {
    MessagePlugin.warning('仅支持 PDF 文件。')
    selectedFile.value = null
    input.value = ''
    return
  }
  selectedFile.value = file
  input.value = ''
}

async function doPreviewParse() {
  if (!selectedFile.value) return

  abortCurrentRequest()

  const requestId = activePreviewRequestId.value + 1
  activePreviewRequestId.value = requestId

  const controller = new AbortController()
  previewAbortController.value = controller

  parsing.value = true
  previewResult.value = null

  try {
    const result = await previewImportFile(
      props.knowledgeBaseId,
      props.setId,
      selectedFile.value,
      parseConfig.value,
      { signal: controller.signal, timeout: 120000 },
    )

    // Guard: ignore stale responses
    if (requestId !== activePreviewRequestId.value) return
    if (!props.visible) return

    previewResult.value = result
  } catch (e: any) {
    // Guard: ignore cancelled requests
    if (controller.signal.aborted) return
    if (e?.name === 'CanceledError' || e?.code === 'ERR_CANCELED') return
    // Guard: ignore stale responses
    if (requestId !== activePreviewRequestId.value) return
    if (!props.visible) return

    MessagePlugin.error(e?.message || '预览请求失败')
  } finally {
    if (requestId === activePreviewRequestId.value) {
      parsing.value = false
      previewAbortController.value = null
    }
  }
}

function editPreviewItem(index: number) {
  const item = previewResult.value?.items?.[index]
  if (!item) return
  editingIndex.value = index
  editingItem.value = { ...item }
  editVisible.value = true
}

function saveEditedItem() {
  if (!previewResult.value || editingIndex.value < 0 || !editingItem.value) return
  const items = previewResult.value.items
  items[editingIndex.value] = { ...editingItem.value }
  editVisible.value = false
  editingItem.value = null
  editingIndex.value = -1
}

function removePreviewItem(index: number) {
  if (!previewResult.value) return
  const items = previewResult.value.items
  items.splice(index, 1)
  if (previewResult.value.stats) {
    previewResult.value.stats.detected_questions = items.length
    let withAns = 0
    let withoutAns = 0
    for (const item of items) {
      if (item.answer_text?.trim()) withAns++
      else withoutAns++
    }
    previewResult.value.stats.with_answer = withAns
    previewResult.value.stats.without_answer = withoutAns
  }
}

async function doConfirmImport() {
  if (!previewResult.value) return
  const items = previewResult.value.items
  if (!items.length) {
    MessagePlugin.warning('没有可导入的题目')
    return
  }

  // Classify and select (respect duplicates)
  const fingerprints = (props.currentQuestions || []).map((q: any) => ({
    question_type: q.question_type,
    stem_text: q.stem_text,
    answer_text: q.answer_text,
  }))
  const classified = classifyQuestionImportItems(items, fingerprints)
  const toImport = selectQuestionImportItems(items, classified, false)

  if (!toImport.length && classified.duplicateItems.length > 0) {
    MessagePlugin.warning('没有可导入的新题，已跳过重复题。')
    return
  }

  const requestId = importingRequestId.value + 1
  importingRequestId.value = requestId

  importing.value = true
  try {
    // Apply import mode status to items
    const itemsWithStatus = toImport.map(item => ({
      ...item,
      status: importMode.value,
    }))
    const response: any = await importQuestions(props.knowledgeBaseId, props.setId, { items: itemsWithStatus })
    const result = response?.data ?? response

    // Guard: ignore response if dialog was closed
    if (requestId !== importingRequestId.value) return
    if (!props.visible) return

    const errors = Array.isArray(result?.errors) ? result.errors : []
    const created = result?.created ?? 0

    if (created > 0) {
      MessagePlugin.success(`成功导入 ${created} 道题目`)
      emit('imported')
    }
    if (errors.length) {
      MessagePlugin.warning(`导入成功 ${created} 道，${errors.length} 道失败`)
    }
    if (created > 0 && !errors.length) {
      // Successful import — close dialog, which triggers closeAndReset
      closeAndReset()
    }
  } catch (e: any) {
    if (requestId !== importingRequestId.value) return
    if (!props.visible) return
    MessagePlugin.error(e?.message || '导入失败')
  } finally {
    if (requestId === importingRequestId.value) {
      importing.value = false
    }
  }
}

// Watch visibility
watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      // Fresh open: ensure clean state
      abortCurrentRequest()
      cleanupDialogState()
    }
    // On close, closeAndReset is already called via @close / cancel button / dialogVisible setter
  },
)

// Watch importType changes — fully reset
watch(
  () => props.importType,
  () => {
    if (props.visible) {
      closeAndReset()
    }
  },
)

onBeforeUnmount(() => {
  abortCurrentRequest()
})
</script>

<style scoped>
.file-upload-area { margin-bottom: 16px; }
.file-upload-label { display: block; }
.file-input { display: none; }
.file-upload-body {
  display: flex; flex-direction: column; align-items: center; justify-content: center;
  gap: 10px; min-height: 120px; border: 1px dashed var(--td-component-stroke);
  border-radius: 6px; color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer); cursor: pointer;
  padding: 16px;
}
.parse-config { margin: 16px 0; }
.config-item { display: flex; align-items: center; gap: 6px; }
.config-label { font-size: 13px; color: var(--td-text-color-secondary); }
.preview-area { margin-top: 16px; }
.preview-stats { margin-bottom: 12px; }
.raw-text { max-height: 200px; overflow-y: auto; font-size: 12px; line-height: 1.6; white-space: pre-wrap; word-break: break-all; background: var(--td-bg-color-secondarycontainer); padding: 12px; border-radius: 4px; }
.question-preview-list { margin-top: 16px; }
.question-preview-list h4 { margin: 0 0 12px; }
.question-preview-item {
  border: 1px solid var(--td-component-stroke); border-radius: 6px; padding: 12px; margin-bottom: 8px;
}
.preview-item-header { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.no-answer-tag { font-size: 12px; color: var(--td-warning-color); }
.preview-item-stem { font-size: 14px; font-weight: 500; margin-bottom: 4px; line-height: 1.5; }
.preview-item-answer { font-size: 13px; color: var(--td-success-color); margin-bottom: 2px; }
.preview-item-answer .answer-label { font-weight: 500; }
.preview-item-analysis { font-size: 13px; color: var(--td-text-color-secondary); }
.preview-item-analysis .analysis-label { font-weight: 500; }
.import-mode-section { margin-top: 16px; padding: 12px; background: var(--td-bg-color-secondarycontainer); border-radius: 6px; }
.import-mode-section .section-title { font-weight: 500; margin-bottom: 8px; }
.import-mode-hint { margin: 8px 0 0; font-size: 12px; color: var(--td-text-color-secondary); }
.warning-text { color: var(--td-warning-color); }
.error-text { color: var(--td-error-color); }
</style>
