<template>
  <t-dialog
    v-model:visible="dialogVisible"
    header="导入题目"
    width="720px"
    :confirm-btn="null"
    :cancel-btn="null"
    :close-on-overlay-click="false"
    @close="closeAndReset"
  >
    <div class="import-layout">
      <!-- Left: import format selector -->
      <div class="left-panel">
        <div class="panel-title">导入格式</div>
        <t-radio-group v-model="importFormat" class="format-group" variant="default-filled" direction="vertical">
          <t-radio value="json" :disabled="parsing">
            <div class="radio-label">
              <span class="radio-title">JSON / JSONL</span>
              <span class="radio-desc">结构化导入</span>
            </div>
          </t-radio>
          <t-radio value="word" :disabled="parsing">
            <div class="radio-label">
              <span class="radio-title">Word / DOCX</span>
              <span class="radio-desc">文档解析导入</span>
            </div>
          </t-radio>
          <t-radio value="pdf" :disabled="parsing">
            <div class="radio-label">
              <span class="radio-title">PDF</span>
              <span class="radio-desc">文档解析导入</span>
            </div>
          </t-radio>
        </t-radio-group>
      </div>

      <!-- Right: context-dependent content -->
      <div class="right-panel">
        <!-- JSON -->
        <template v-if="importFormat === 'json'">
          <div class="json-notice">
            <t-alert theme="info" :close-btn="false">
              JSON / JSONL 导入暂不支持工作台模式，请使用原导入流程。
            </t-alert>
          </div>
        </template>

        <!-- Word / PDF -->
        <template v-else>
          <div class="file-upload-area">
            <label class="file-upload-label">
              <input ref="fileInputRef" type="file" :accept="accept" class="file-input" @change="onFileSelected" />
              <div class="file-upload-body">
                <t-icon name="upload" size="24px" />
                <span v-if="selectedFile">{{ selectedFile.name }} ({{ formatFileSize(selectedFile.size) }})</span>
                <span v-else>选择或拖拽文件</span>
                <t-button size="small" variant="outline" @click.stop="fileInputRef?.click()">选择文件</t-button>
              </div>
            </label>
          </div>

          <div class="config-row">
            <div class="config-item">
              <span class="config-label">默认难度</span>
              <t-select v-model="parseConfig.default_difficulty" style="width: 100px" size="small">
                <t-option value="easy" label="简单" />
                <t-option value="medium" label="中等" />
                <t-option value="hard" label="困难" />
              </t-select>
            </div>
            <div class="config-item" v-if="availablePresets.length > 1">
              <span class="config-label">分块策略</span>
              <t-select v-model="parseConfig.strategy_preset" style="width: 120px" size="small">
                <t-option v-for="p in availablePresets" :key="p.value" :value="p.value" :label="p.label" />
              </t-select>
            </div>
          </div>

          <div v-if="previewError" class="preview-error">
            <t-alert theme="error" :close-btn="false">{{ previewError }}</t-alert>
          </div>

        </template>

        <div class="action-bar">
          <t-button variant="outline" @click="closeAndReset">取消</t-button>
          <t-button
            v-if="importFormat !== 'json'"
            theme="primary"
            :loading="parsing"
            :disabled="!selectedFile || parsing"
            @click="handleStartParsing"
          >
            开始解析
          </t-button>
        </div>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { previewImportBlocks } from '@/api/question_block'
import { saveDraft } from '@/utils/importDraftDB'
import { useImportWorkbenchStore } from '@/stores/importWorkbench'

const router = useRouter()
const workbenchStore = useImportWorkbenchStore()

const props = withDefaults(defineProps<{
  visible: boolean
  knowledgeBaseId: string
  setId: string
  importMode?: 'single' | 'batch'
}>(), { importMode: 'single' })

const emit = defineEmits<{ 'update:visible': [value: boolean] }>()

const dialogVisible = computed({
  get: () => props.visible,
  set: (value: boolean) => { if (!value) closeAndReset() },
})

const accept = computed(() => {
  if (importFormat.value === 'word') return '.doc,.docx'
  if (importFormat.value === 'pdf') return '.pdf'
  return ''
})

const availablePresets = computed(() => {
  if (importFormat.value === 'pdf') {
    return [
      { value: 'general', label: 'General' },
      { value: 'pdf', label: 'PDF' },
    ]
  }
  return [{ value: 'general', label: 'General' }]
})

const fileInputRef = ref<HTMLInputElement | null>(null)
const selectedFile = ref<File | null>(null)
const parsing = ref(false)
const previewError = ref('')
const importFormat = ref<'json' | 'word' | 'pdf'>('word')

const parseConfig = ref({
  default_difficulty: 'medium',
  strategy_preset: 'general',
})

watch(() => props.visible, (visible) => {
  if (visible) {
    importFormat.value = 'word'
  }
})

watch(importFormat, (format) => {
  selectedFile.value = null
  previewError.value = ''
  parseConfig.value.strategy_preset = format === 'pdf' ? 'pdf' : 'general'
  if (fileInputRef.value) fileInputRef.value.value = ''
})

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

function onFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  previewError.value = ''
  selectedFile.value = file
  input.value = ''
  if (importFormat.value === 'pdf') {
    parseConfig.value.strategy_preset = 'pdf'
  }
}

function closeAndReset() {
  selectedFile.value = null
  parsing.value = false
  previewError.value = ''
  parseConfig.value = { default_difficulty: 'medium', strategy_preset: 'general' }
  if (fileInputRef.value) fileInputRef.value.value = ''
  emit('update:visible', false)
}

async function handleStartParsing() {
  if (!selectedFile.value) return
  parsing.value = true
  previewError.value = ''

  try {
    const result = await previewImportBlocks(
      props.knowledgeBaseId,
      props.setId,
      selectedFile.value,
      {
        default_difficulty: parseConfig.value.default_difficulty,
        strategy_preset: parseConfig.value.strategy_preset,
        import_mode: props.importMode,
      },
      { timeout: 120000 },
    )

    if (!result.blocks || result.blocks.length === 0) {
      previewError.value = '未识别到题目块，请检查文件格式。'
      parsing.value = false
      return
    }

    workbenchStore.kbId = props.knowledgeBaseId
    workbenchStore.setId = props.setId
    workbenchStore.strategyPreset = parseConfig.value.strategy_preset
    workbenchStore.defaultDifficulty = parseConfig.value.default_difficulty
    workbenchStore.importMode = props.importMode
    workbenchStore.importFormat = importFormat.value
    workbenchStore.setBlocksFromResponse(result.blocks)

    await saveDraft({
      kbId: props.knowledgeBaseId,
      setId: props.setId,
      blocks: result.blocks,
      strategyPreset: parseConfig.value.strategy_preset,
      defaultDifficulty: parseConfig.value.default_difficulty,
      importMode: props.importMode,
      importFormat: importFormat.value,
      currentStep: 'block-review',
      questions: [],
      timestamp: Date.now(),
    })

    emit('update:visible', false)

    // Fix 3: use named route
    router.push({
      name: 'questionImportWorkbench',
      params: { kbId: props.knowledgeBaseId, setId: props.setId },
    })
  } catch (e: any) {
    if (e?.name === 'CanceledError' || e?.code === 'ERR_CANCELED') return
    previewError.value = e?.message || '解析失败，请重试'
  } finally {
    parsing.value = false
  }
}
</script>

<style scoped>
.import-layout { display: flex; gap: 24px; min-height: 280px; }
.left-panel { width: 160px; flex-shrink: 0; border-right: 1px solid var(--td-component-stroke); padding-right: 16px; }
.left-panel .panel-title { font-weight: 500; margin-bottom: 12px; font-size: 14px; }
.format-group { display: flex; flex-direction: column; gap: 8px; }
.radio-label { display: flex; flex-direction: column; }
.radio-title { font-weight: 500; font-size: 13px; }
.radio-desc { font-size: 11px; color: var(--td-text-color-placeholder); }
.right-panel { flex: 1; display: flex; flex-direction: column; gap: 16px; }
.json-notice { flex: 1; display: flex; align-items: center; }
.file-upload-label { display: block; }
.file-input { display: none; }
.file-upload-body { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; min-height: 100px; border: 1px dashed var(--td-component-stroke); border-radius: 6px; color: var(--td-text-color-secondary); background: var(--td-bg-color-secondarycontainer); cursor: pointer; padding: 12px; }
.config-row { display: flex; gap: 16px; flex-wrap: wrap; }
.config-item { display: flex; align-items: center; gap: 6px; }
.config-label { font-size: 13px; color: var(--td-text-color-secondary); white-space: nowrap; }
.action-bar { display: flex; justify-content: flex-end; gap: 8px; margin-top: auto; padding-top: 8px; }
</style>
