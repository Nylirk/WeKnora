<template>
  <t-dialog
    :visible="visible"
    :header="false"
    :footer="false"
    :close-btn="false"
    width="560px"
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
            <span class="upload-icon"><t-icon name="upload" size="20px" /></span>
            <span class="upload-name">{{ selectedFile ? selectedFile.name : '选择或拖拽文件' }}</span>
            <span class="upload-hint">{{ selectedFile ? formatFileSize(selectedFile.size) : acceptHint }}</span>
            <t-button size="small" variant="outline" @click.prevent="fileInputRef?.click()">选择文件</t-button>
          </label>

          <div v-if="importFormat === 'pdf'" class="preset-row">
            <span class="config-label">分块策略</span>
            <t-select v-model="strategyPreset" size="small" style="width: 128px">
              <t-option value="general" label="general" />
              <t-option value="pdf" label="pdf" />
            </t-select>
          </div>

          <t-alert v-if="previewError" theme="error" :close-btn="false" class="preview-error">{{ previewError }}</t-alert>
        </div>
      </div>

      <!-- Fix 7: buttons at bottom-right -->
      <div class="dialog-footer">
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
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useImportUIStore } from '@/stores/importUIStore'
import { previewImportBlocks, type BlockPreviewSummary, type ImportBlock } from '@/api/question_block'

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

const formatOptions: Array<{ value: ImportFormat; title: string; description: string; disabled?: boolean }> = [
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

watch(() => props.visible, (visible) => { if (visible) resetState() })

function selectFormat(format: ImportFormat) {
  if (format === 'json') return
  importFormat.value = format; strategyPreset.value = format === 'pdf' ? 'pdf' : 'general'
  selectedFile.value = null; previewError.value = ''
  if (fileInputRef.value) fileInputRef.value.value = ''
}
function formatFileSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}
function onFileSelected(event: Event) {
  const input = event.target as HTMLInputElement; const file = input.files?.[0]
  if (!file) return; selectedFile.value = file; previewError.value = ''; input.value = ''
}
function resetState() {
  selectedFile.value = null; parsing.value = false; previewError.value = ''
  importFormat.value = 'word'; strategyPreset.value = 'general'
  if (fileInputRef.value) fileInputRef.value.value = ''
}
function closeAndReset() { resetState(); emit('update:visible', false) }
function handleVisibleUpdate(value: boolean) { if (!value) closeAndReset() }

async function handleStartParsing() {
  if (!selectedFile.value || importFormat.value === 'json') return
  parsing.value = true; previewError.value = ''

  await importUI.withImportLoading('正在解析文件…', async () => {
    const result = await previewImportBlocks(props.knowledgeBaseId, props.setId, selectedFile.value!, {
      default_difficulty: 'medium', strategy_preset: strategyPreset.value, import_mode: props.importMode,
    }, { timeout: 120000 })
    const blocks = Array.isArray(result.blocks) ? result.blocks : []
    if (blocks.length === 0) { previewError.value = '未识别到题目块，请检查文件格式。'; return }
    emit('parsed', { blocks, summary: result.summary, strategyPreset: strategyPreset.value, importFormat: importFormat.value, importMode: props.importMode })
  }).catch((error: any) => {
    if (error?.name === 'CanceledError' || error?.code === 'ERR_CANCELED') return
    previewError.value = error?.message || '解析失败，请重试'
  })

  parsing.value = false
}
</script>

<style scoped>
.dialog-shell { padding: 0; }
.dialog-topbar { padding-bottom: 12px; border-bottom: 1px solid var(--td-component-stroke); }
.dialog-topbar h3 { margin: 0; font-size: 17px; line-height: 24px; }
.dialog-topbar p { margin: 2px 0 0; font-size: 12px; color: var(--td-text-color-secondary); }
.import-layout { display: flex; gap: 14px; padding-top: 12px; }
.format-panel { width: 140px; flex-shrink: 0; display: flex; flex-direction: column; gap: 5px; }
.panel-title { font-size: 11px; font-weight: 500; color: var(--td-text-color-secondary); margin-bottom: 2px; }
.format-pill { width: 100%; display: flex; flex-direction: column; gap: 1px; padding: 7px 10px; border: 1px solid transparent; border-radius: 10px; background: var(--td-bg-color-secondarycontainer); text-align: left; cursor: pointer; transition: border-color .15s, background-color .15s; }
.format-pill:not(:disabled):hover { border-color: var(--td-brand-color-light); background: var(--td-brand-color-light); }
.format-pill.selected { border-color: var(--td-brand-color); background: var(--td-brand-color-light); box-shadow: inset 3px 0 0 var(--td-brand-color); }
.format-pill:disabled { opacity: .65; cursor: not-allowed; }
.format-title-row { display: flex; align-items: center; justify-content: space-between; gap: 4px; }
.format-title { font-size: 12px; font-weight: 600; }
.format-desc { font-size: 10px; color: var(--td-text-color-secondary); }
.coming-soon { padding: 1px 4px; border-radius: 6px; background: var(--td-bg-color-container); font-size: 9px; white-space: nowrap; }
.upload-panel { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 8px; }
.file-upload-label { min-height: 96px; box-sizing: border-box; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 4px; padding: 10px; border: 1px dashed var(--td-component-stroke); border-radius: 8px; background: var(--td-bg-color-secondarycontainer); cursor: pointer; }
.file-upload-label:hover { border-color: var(--td-brand-color); background: var(--td-brand-color-light); }
.file-input { display: none; }
.upload-icon { color: var(--td-brand-color); }
.upload-name { font-size: 12px; font-weight: 500; }
.upload-hint { font-size: 11px; color: var(--td-text-color-placeholder); }
.preset-row { display: flex; align-items: center; justify-content: flex-end; gap: 8px; }
.config-label { font-size: 12px; color: var(--td-text-color-secondary); }
/* Fix 7: footer at bottom-right */
.dialog-footer { display: flex; justify-content: flex-end; gap: 8px; padding-top: 14px; border-top: 1px solid var(--td-component-stroke); margin-top: 12px; }
</style>
