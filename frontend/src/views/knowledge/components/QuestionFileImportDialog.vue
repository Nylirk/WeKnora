<template>
  <t-dialog
    :visible="visible"
    :header="false"
    :footer="false"
    :close-btn="false"
    width="600px"
    top="14vh"
    :z-index="2500"
    dialog-class-name="question-file-import-dialog"
    :close-on-overlay-click="false"
    :close-on-esc-keydown="!parsing"
    @update:visible="handleVisibleUpdate"
  >
    <div class="dialog-shell">
      <div class="dialog-topbar">
        <div>
          <h3>导入题目</h3>
          <p>选择文件格式并开始解析</p>
        </div>
        <div class="dialog-actions">
          <t-button size="small" variant="outline" :disabled="parsing" @click="closeAndReset">取消</t-button>
          <t-button
            size="small"
            theme="primary"
            :loading="parsing"
            :disabled="importFormat === 'json' || !selectedFile || parsing"
            @click="handleStartParsing"
          >
            开始解析
          </t-button>
        </div>
      </div>

      <div class="import-layout">
        <div class="format-panel">
          <div class="panel-title">导入格式</div>
          <button
            v-for="format in formatOptions"
            :key="format.value"
            type="button"
            class="format-pill"
            :class="{ selected: importFormat === format.value }"
            :disabled="parsing || format.disabled"
            @click="selectFormat(format.value)"
          >
            <span class="format-title-row">
              <span class="format-title">{{ format.title }}</span>
              <span v-if="format.disabled" class="coming-soon">即将支持</span>
            </span>
            <span class="format-desc">{{ format.description }}</span>
          </button>
        </div>

        <div class="upload-panel">
          <label class="file-upload-label">
            <input ref="fileInputRef" type="file" :accept="accept" class="file-input" @change="onFileSelected" />
            <span class="upload-icon"><t-icon name="upload" size="22px" /></span>
            <span class="upload-name">
              {{ selectedFile ? selectedFile.name : '选择或拖拽文件' }}
            </span>
            <span class="upload-hint">
              {{ selectedFile ? formatFileSize(selectedFile.size) : acceptHint }}
            </span>
            <t-button size="small" variant="outline" @click.prevent="fileInputRef?.click()">选择文件</t-button>
          </label>

          <div v-if="importFormat === 'pdf'" class="preset-row">
            <span class="config-label">分块策略</span>
            <t-select v-model="strategyPreset" size="small" style="width: 128px">
              <t-option value="general" label="general" />
              <t-option value="pdf" label="pdf" />
            </t-select>
          </div>

          <t-alert v-if="previewError" theme="error" :close-btn="false" class="preview-error">
            {{ previewError }}
          </t-alert>
        </div>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { previewImportBlocks, type BlockPreviewSummary, type ImportBlock } from '@/api/question_block'

type ImportFormat = 'json' | 'word' | 'pdf'

interface ParsedPayload {
  blocks: ImportBlock[]
  summary: BlockPreviewSummary
  strategyPreset: string
  importFormat: ImportFormat
  importMode: 'single' | 'batch'
}

const props = withDefaults(defineProps<{
  visible: boolean
  knowledgeBaseId: string
  setId: string
  importMode?: 'single' | 'batch'
}>(), { importMode: 'single' })

const emit = defineEmits<{
  'update:visible': [value: boolean]
  parsed: [payload: ParsedPayload]
}>()

const formatOptions: Array<{
  value: ImportFormat
  title: string
  description: string
  disabled?: boolean
}> = [
  { value: 'json', title: 'JSON / JSONL', description: '结构化导入', disabled: true },
  { value: 'word', title: 'Word / DOCX', description: '文档解析导入' },
  { value: 'pdf', title: 'PDF', description: '文档解析导入' },
]

const fileInputRef = ref<HTMLInputElement | null>(null)
const selectedFile = ref<File | null>(null)
const parsing = ref(false)
const previewError = ref('')
const importFormat = ref<ImportFormat>('word')
const strategyPreset = ref('general')

const accept = computed(() => importFormat.value === 'pdf' ? '.pdf' : '.doc,.docx')
const acceptHint = computed(() => importFormat.value === 'pdf' ? '支持 PDF' : '支持 DOC / DOCX')

watch(() => props.visible, (visible) => {
  if (visible) resetState()
})

function selectFormat(format: ImportFormat) {
  if (format === 'json') return
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

function resetState() {
  selectedFile.value = null
  parsing.value = false
  previewError.value = ''
  importFormat.value = 'word'
  strategyPreset.value = 'general'
  if (fileInputRef.value) fileInputRef.value.value = ''
}

function closeAndReset() {
  resetState()
  emit('update:visible', false)
}

function handleVisibleUpdate(value: boolean) {
  if (!value) closeAndReset()
}

async function handleStartParsing() {
  if (!selectedFile.value || importFormat.value === 'json') return
  parsing.value = true
  previewError.value = ''

  try {
    const result = await previewImportBlocks(
      props.knowledgeBaseId,
      props.setId,
      selectedFile.value,
      {
        default_difficulty: 'medium',
        strategy_preset: strategyPreset.value,
        import_mode: props.importMode,
      },
      { timeout: 120000 },
    )

    const blocks = Array.isArray(result.blocks) ? result.blocks : []
    if (blocks.length === 0) {
      previewError.value = '未识别到题目块，请检查文件格式。'
      return
    }

    emit('parsed', {
      blocks,
      summary: result.summary,
      strategyPreset: strategyPreset.value,
      importFormat: importFormat.value,
      importMode: props.importMode,
    })
  } catch (error: any) {
    if (error?.name === 'CanceledError' || error?.code === 'ERR_CANCELED') return
    previewError.value = error?.message || '解析失败，请重试'
  } finally {
    parsing.value = false
  }
}
</script>

<style scoped>
.dialog-shell { padding: 2px 0 4px; }
.dialog-topbar { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding-bottom: 14px; border-bottom: 1px solid var(--td-component-stroke); }
.dialog-topbar h3 { margin: 0; font-size: 18px; line-height: 26px; color: var(--td-text-color-primary); }
.dialog-topbar p { margin: 2px 0 0; font-size: 12px; color: var(--td-text-color-secondary); }
.dialog-actions { display: flex; gap: 8px; padding-top: 1px; }
.import-layout { display: flex; gap: 18px; padding-top: 16px; }
.format-panel { width: 150px; flex-shrink: 0; display: flex; flex-direction: column; gap: 7px; }
.panel-title { margin-bottom: 1px; font-size: 12px; font-weight: 500; color: var(--td-text-color-secondary); }
.format-pill { width: 100%; display: flex; flex-direction: column; gap: 2px; padding: 9px 11px; border: 1px solid transparent; border-radius: 12px; background: var(--td-bg-color-secondarycontainer); color: var(--td-text-color-primary); text-align: left; cursor: pointer; transition: border-color .15s, background-color .15s, color .15s; }
.format-pill:not(:disabled):hover { border-color: var(--td-brand-color-light); background: var(--td-brand-color-light); }
.format-pill.selected { border-color: var(--td-brand-color); background: var(--td-brand-color-light); color: var(--td-brand-color); box-shadow: inset 3px 0 0 var(--td-brand-color); }
.format-pill:disabled { color: var(--td-text-color-disabled); background: var(--td-bg-color-secondarycontainer); cursor: not-allowed; opacity: .72; }
.format-title-row { display: flex; align-items: center; justify-content: space-between; gap: 6px; width: 100%; }
.format-title { font-size: 13px; font-weight: 600; }
.format-desc { font-size: 11px; color: var(--td-text-color-secondary); }
.format-pill.selected .format-desc { color: var(--td-brand-color); opacity: .82; }
.coming-soon { padding: 1px 4px; border-radius: 8px; background: var(--td-bg-color-container); font-size: 9px; font-weight: 500; white-space: nowrap; }
.upload-panel { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 10px; }
.file-upload-label { min-height: 112px; box-sizing: border-box; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 5px; padding: 12px; border: 1px dashed var(--td-component-stroke); border-radius: 10px; background: var(--td-bg-color-secondarycontainer); color: var(--td-text-color-secondary); cursor: pointer; }
.file-upload-label:hover { border-color: var(--td-brand-color); background: var(--td-brand-color-light); }
.file-input { display: none; }
.upload-icon { color: var(--td-brand-color); line-height: 1; }
.upload-name { max-width: 100%; overflow: hidden; color: var(--td-text-color-primary); font-size: 13px; font-weight: 500; text-overflow: ellipsis; white-space: nowrap; }
.upload-hint { font-size: 11px; color: var(--td-text-color-placeholder); }
.preset-row { display: flex; align-items: center; justify-content: flex-end; gap: 8px; }
.config-label { font-size: 12px; color: var(--td-text-color-secondary); }
.preview-error { margin-top: 2px; }
</style>
