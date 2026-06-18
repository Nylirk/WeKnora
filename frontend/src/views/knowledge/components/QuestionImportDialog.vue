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
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { importQuestions, type ImportQuestionError, type Question } from '@/api/question'
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

async function doImport() {
  backendErrors.value = []
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
    MessagePlugin.success(`成功导入 ${result?.created || 0} 道题目`)
    const missingAnswerCount = items.filter(item => !item.answer_text.trim()).length
    if (missingAnswerCount) {
      MessagePlugin.warning(`${missingAnswerCount} 道题缺少答案，审核或导出前需要补全`)
    }
    if ((result?.created || 0) > 0) emit('imported')
    if (parseErrors.value.length === 0) dialogVisible.value = false
  } catch (e: any) {
    MessagePlugin.error(e?.message || '导入失败')
  } finally {
    importing.value = false
  }
}

watch(() => props.visible, visible => {
  if (!visible) return
  importMode.value = 'paste'
  pastedData.value = ''
  fileData.value = ''
  selectedFileName.value = ''
  backendErrors.value = []
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
</style>
