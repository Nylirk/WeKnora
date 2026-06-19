<template>
  <t-dialog
    :visible="visible"
    :header="false"
    :footer="false"
    :close-btn="false"
    width="580px"
    top="10vh"
    :z-index="2500"
    dialog-class-name="question-file-import-dialog"
    :close-on-overlay-click="false"
    :close-on-esc-keydown="!parsing"
    @update:visible="handleVisibleUpdate"
  >
    <div class="dialog-shell">
      <!-- Header -->
      <div class="dialog-topbar">
        <h3>导入题目</h3>
        <p>上传 JSON / JSONL、Word / DOCX 或 PDF 文件，解析后进入导入工作台</p>
      </div>

      <!-- P0: horizontal format cards -->
      <div class="format-cards">
        <button
          v-for="format in formatOptions"
          :key="format.value"
          type="button"
          class="format-card"
          :class="{ selected: importFormat === format.value }"
          :disabled="parsing"
          @click="selectFormat(format.value)"
        >
          <span class="format-card-title">{{ format.title }}</span>
          <span class="format-card-desc">{{ format.description }}</span>
          <span class="format-card-ext">{{ format.extensions }}</span>
        </button>
      </div>

      <!-- Upload area -->
      <label class="file-upload-label">
        <input ref="fileInputRef" type="file" :accept="accept" class="file-input" @change="onFileSelected" />
        <span class="upload-icon"><t-icon name="upload" size="22px" /></span>
        <span class="upload-name">{{ selectedFile ? selectedFile.name : '选择或拖拽文件' }}</span>
        <span class="upload-hint">{{ selectedFile ? formatFileSize(selectedFile.size) : acceptHint }}</span>
        <t-button v-if="!selectedFile" size="small" variant="outline" @click.prevent="fileInputRef?.click()">选择文件</t-button>
        <t-button v-else size="small" variant="text" @click.prevent="clearFile">重新选择</t-button>
      </label>

      <!-- PDF preset only -->
      <div v-if="importFormat === 'pdf'" class="preset-row">
        <span class="config-label">分块策略</span>
        <t-select v-model="strategyPreset" size="small" style="width: 128px">
          <t-option value="general" label="general" />
          <t-option value="pdf" label="pdf" />
        </t-select>
      </div>

      <t-alert v-if="previewError" theme="error" :close-btn="false" class="preview-error">{{ previewError }}</t-alert>

      <!-- Footer -->
      <div class="dialog-footer">
        <t-button size="small" variant="outline" :disabled="parsing" @click="closeAndReset">取消</t-button>
        <t-button
          size="small"
          theme="primary"
          :loading="parsing"
          :disabled="!selectedFile || parsing"
          @click="handleStartParsing"
        >
          开始解析
        </t-button>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useImportUIStore } from '@/stores/importUIStore'
import { previewImportBlocks, type BlockPreviewSummary, type ImportBlock } from '@/api/question_block'
import { parseJsonQuestionFileToBlocks } from '@/utils/jsonQuestionImport'

type ImportFormat = 'json' | 'word' | 'pdf'

interface ParsedPayload {
  blocks: ImportBlock[]
  summary: BlockPreviewSummary
  strategyPreset: string
  importFormat: ImportFormat
  importMode: 'single' | 'batch'
}

const props = withDefaults(defineProps<{ visible: boolean; knowledgeBaseId: string; setId: string; importMode?: 'single' | 'batch' }>(), { importMode: 'single' })
const emit = defineEmits<{ 'update:visible': [value: boolean]; parsed: [payload: ParsedPayload] }>()
const importUI = useImportUIStore()

const formatOptions: Array<{ value: ImportFormat; title: string; description: string; extensions: string }> = [
  { value: 'json', title: 'JSON / JSONL', description: '结构化题目数据', extensions: '.json / .jsonl' },
  { value: 'word', title: 'Word / DOCX', description: '从文档中识别题目结构', extensions: '.doc / .docx' },
  { value: 'pdf', title: 'PDF', description: '从 PDF 中识别题目结构', extensions: '.pdf' },
]

const fileInputRef = ref<HTMLInputElement | null>(null)
const selectedFile = ref<File | null>(null)
const parsing = ref(false)
const previewError = ref('')
const importFormat = ref<ImportFormat>('json')
const strategyPreset = ref('pdf')

const accept = computed(() => {
  if (importFormat.value === 'json') return '.json,.jsonl'
  if (importFormat.value === 'pdf') return '.pdf'
  return '.doc,.docx'
})
const acceptHint = computed(() => {
  if (importFormat.value === 'json') return '支持 JSON / JSONL'
  if (importFormat.value === 'pdf') return '支持 PDF'
  return '支持 DOC / DOCX'
})

watch(() => props.visible, (visible) => { if (visible) resetState() })

function selectFormat(format: ImportFormat) {
  importFormat.value = format
  strategyPreset.value = format === 'pdf' ? 'pdf' : 'general'
  selectedFile.value = null
  previewError.value = ''
  if (fileInputRef.value) fileInputRef.value.value = ''
}
function formatFileSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}
function onFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  selectedFile.value = file
  previewError.value = ''
  input.value = ''
}
function clearFile() {
  selectedFile.value = null
  previewError.value = ''
  if (fileInputRef.value) fileInputRef.value.value = ''
}
function resetState() {
  selectedFile.value = null
  parsing.value = false
  previewError.value = ''
  importFormat.value = 'json'
  strategyPreset.value = 'pdf'
  if (fileInputRef.value) fileInputRef.value.value = ''
}
function closeAndReset() { resetState(); emit('update:visible', false) }
function handleVisibleUpdate(value: boolean) { if (!value) closeAndReset() }

async function handleStartParsing() {
  if (!selectedFile.value) return
  parsing.value = true
  previewError.value = ''

  const format = importFormat.value
  const file = selectedFile.value!

  if (format === 'json') {
    // P1+P3: JSON/JSONL → parse locally, emit to workbench
    await importUI.withImportLoading('正在解析 JSON 题目…', async () => {
      try {
        const result = await parseJsonQuestionFileToBlocks(file)
        if (result.blocks.length === 0) {
          previewError.value = '未识别到题目数据，请检查 JSON / JSONL 格式。'
          return
        }
        emit('parsed', {
          blocks: result.blocks,
          summary: result.summary,
          strategyPreset: 'json',
          importFormat: 'json',
          importMode: props.importMode,
        })
      } catch (e: any) {
        previewError.value = e?.message || 'JSON 解析失败，请检查文件格式'
      }
    })
  } else {
    // Word / PDF → call previewImportBlocks API (doc reader)
    await importUI.withImportLoading('正在解析文件…', async () => {
      try {
        const result = await previewImportBlocks(props.knowledgeBaseId, props.setId, file, {
          default_difficulty: 'medium',
          strategy_preset: strategyPreset.value,
          import_mode: props.importMode,
        }, { timeout: 120000 })
        const blocks = Array.isArray(result.blocks) ? result.blocks : []
        if (blocks.length === 0) {
          previewError.value = '未识别到题目块，请检查文件格式。'
          return
        }
        emit('parsed', {
          blocks,
          summary: result.summary,
          strategyPreset: strategyPreset.value,
          importFormat: format,
          importMode: props.importMode,
        })
      } catch (error: any) {
        if (error?.name === 'CanceledError' || error?.code === 'ERR_CANCELED') return
        previewError.value = error?.message || '解析失败，请重试'
      }
    })
  }

  parsing.value = false
}
</script>

<style scoped>
.dialog-shell { padding: 0; }
.dialog-topbar { padding-bottom: 14px; border-bottom: 1px solid var(--td-component-stroke); }
.dialog-topbar h3 { margin: 0; font-size: 17px; line-height: 24px; }
.dialog-topbar p { margin: 4px 0 0; font-size: 12px; color: var(--td-text-color-secondary); }

/* P0: horizontal format cards */
.format-cards {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
  padding-top: 14px;
}
.format-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 14px 10px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 12px;
  background: var(--td-bg-color-container);
  text-align: center;
  cursor: pointer;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.format-card:hover:not(:disabled) {
  border-color: var(--td-brand-color);
  box-shadow: 0 0 0 2px var(--td-brand-color-light);
}
.format-card.selected {
  border-color: var(--td-brand-color);
  box-shadow: 0 0 0 2px var(--td-brand-color-light);
  background: var(--td-brand-color-light);
}
.format-card:disabled { opacity: 0.55; cursor: not-allowed; }
.format-card-title { font-size: 13px; font-weight: 600; }
.format-card-desc { font-size: 11px; color: var(--td-text-color-secondary); }
.format-card-ext { font-size: 10px; color: var(--td-text-color-placeholder); }

/* Upload area */
.file-upload-label {
  min-height: 100px;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 14px 10px;
  margin-top: 12px;
  border: 1px dashed var(--td-component-stroke);
  border-radius: 10px;
  background: var(--td-bg-color-secondarycontainer);
  cursor: pointer;
}
.file-upload-label:hover { border-color: var(--td-brand-color); background: var(--td-brand-color-light); }
.file-input { display: none; }
.upload-icon { color: var(--td-brand-color); }
.upload-name { font-size: 13px; font-weight: 500; }
.upload-hint { font-size: 11px; color: var(--td-text-color-placeholder); }

/* PDF preset */
.preset-row {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 10px;
}
.config-label { font-size: 12px; color: var(--td-text-color-secondary); }

/* Error */
.preview-error { margin-top: 10px; }

/* Footer */
.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 14px;
  margin-top: 12px;
  border-top: 1px solid var(--td-component-stroke);
}
</style>
