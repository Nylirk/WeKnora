<template>
  <div class="block-review-panel">
    <div class="toolbar">
      <t-space>
        <t-select v-model="store.anomalyFilter" size="small" style="width: 120px">
          <t-option value="all" label="全部" />
          <t-option value="error" label="错误" />
          <t-option value="warning" label="警告" />
        </t-select>
        <t-button size="small" variant="outline" :disabled="importUI.loading" @click="emit('sort')">按题号排序</t-button>
        <t-button size="small" variant="outline" theme="warning" :disabled="!store.hasDeletedBlocks || importUI.loading" @click="emit('restore-deleted')">恢复删除</t-button>
      </t-space>
      <span class="block-count">{{ store.filteredBlocks.length }} / {{ store.blocks.length }} blocks</span>
    </div>

    <div class="review-body">
      <div class="col-list">
        <div
          v-for="block in store.filteredBlocks"
          :key="block.id"
          class="block-item"
          :class="{ selected: store.selectedBlockId === block.id, 'has-error': hasError(block) }"
          @click="store.selectBlock(block.id)"
        >
          <div class="block-item-header">
            <t-tag v-if="block.question_number != null" size="small" theme="primary" variant="light">#{{ block.question_number }}</t-tag>
            <t-tag v-else size="small" theme="default">无题号</t-tag>
            <span class="block-item-index">idx {{ block.index }}</span>
            <t-space size="2px">
              <span v-for="a in safeAnomalies(block)" :key="a.code">
                <t-tooltip :content="a.message">
                  <t-tag size="small" :theme="a.severity === 'error' ? 'danger' : 'warning'" variant="light">{{ a.code }}</t-tag>
                </t-tooltip>
              </span>
            </t-space>
          </div>
          <div class="block-item-preview">{{ block.current_text.slice(0, 100) }}{{ block.current_text.length > 100 ? '…' : '' }}</div>
        </div>
        <t-empty v-if="store.filteredBlocks.length === 0" description="无 blocks" />
      </div>

      <div class="col-editor" v-if="store.selectedBlock">
        <div class="detail-toolbar">
          <t-space size="small">
            <t-button size="small" variant="outline" :disabled="importUI.loading" @click="emit('restore-original', store.selectedBlock!.id)">恢复原始文本</t-button>
            <t-button size="small" variant="outline" :disabled="importUI.loading" @click="doSplit">拆分</t-button>
            <t-button size="small" variant="outline" :disabled="importUI.loading" @click="emit('merge-previous', store.selectedBlock!.id)">合并上一个</t-button>
            <t-button size="small" variant="outline" :disabled="importUI.loading" @click="emit('merge-next', store.selectedBlock!.id)">合并下一个</t-button>
            <t-button size="small" variant="outline" theme="danger" :disabled="importUI.loading" @click="emit('delete-block', store.selectedBlock!.id)">删除</t-button>
          </t-space>
        </div>
        <t-textarea v-model="editingText" :autosize="{ minRows: 6, maxRows: 20 }" @change="onTextChange" />
        <div class="split-control" v-if="showSplitControl">
          <span class="split-hint">输入拆分关键词（如 "249"）：</span>
          <t-input v-model="splitKeyword" size="small" style="width: 120px" placeholder="如: 249" @enter="doSplitByKeyword" />
          <t-button size="small" :disabled="importUI.loading" @click="doSplitByKeyword">执行拆分</t-button>
        </div>
      </div>
      <t-empty v-else description="选择一个 block" class="col-editor col-editor-empty" />

      <aside class="col-meta" v-if="store.selectedBlock">
        <section class="meta-section">
          <h4>标签</h4>
          <div class="tag-edit-list">
            <div v-for="(tag, i) in selectedBlockTags" :key="i" class="tag-edit-row">
              <t-tag size="small" variant="outline" class="tag-edit-text">{{ tag }}</t-tag>
              <t-button size="small" variant="text" theme="danger" :disabled="importUI.loading" @click="emit('remove-tag', { id: store.selectedBlock!.id, tag })">
                <t-icon name="close" size="12px" />
              </t-button>
            </div>
          </div>
          <div class="tag-add-row">
            <t-input v-model="newTag" size="small" placeholder="添加标签" @enter="addTag" style="flex:1" />
            <t-button size="small" variant="outline" :disabled="importUI.loading" @click="addTag">添加</t-button>
          </div>
        </section>
        <section class="meta-section">
          <h4>异常信息</h4>
          <div v-if="selectedBlockAnomalies.length" class="detail-anomalies">
            <div v-for="a in selectedBlockAnomalies" :key="a.code" class="anomaly-line" :class="a.severity">
              <span class="anomaly-code">{{ a.code }}</span>
              <span>{{ a.message }}</span>
            </div>
          </div>
          <span v-else class="meta-empty">无异常</span>
        </section>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useImportWorkbenchStore } from '@/stores/importWorkbench'
import { useImportUIStore } from '@/stores/importUIStore'
import type { ImportBlock } from '@/api/question_block'

const store = useImportWorkbenchStore()
const importUI = useImportUIStore()
const emit = defineEmits<{
  changed: []
  sort: []
  'restore-original': [id: string]
  'split-block': [payload: { id: string; positions: number[] }]
  'merge-previous': [id: string]
  'merge-next': [id: string]
  'delete-block': [id: string]
  'restore-deleted': []
  'add-tag': [payload: { id: string; tag: string }]
  'remove-tag': [payload: { id: string; tag: string }]
}>()

const editingText = ref('')
const showSplitControl = ref(false)
const splitKeyword = ref('')
const newTag = ref('')

const selectedBlockAnomalies = computed(() =>
  Array.isArray(store.selectedBlock?.anomalies) ? store.selectedBlock.anomalies : []
)
const selectedBlockTags = computed(() =>
  Array.isArray(store.selectedBlock?.tags) ? store.selectedBlock.tags : []
)

function safeAnomalies(block: ImportBlock) { return Array.isArray(block.anomalies) ? block.anomalies : [] }
function hasError(block: ImportBlock) { return safeAnomalies(block).some(a => a?.severity === 'error') }

watch(() => [store.selectedBlock?.id, store.selectedBlock?.current_text] as const, ([, ct]) => {
  editingText.value = ct ?? ''; showSplitControl.value = false; splitKeyword.value = ''; newTag.value = ''
}, { immediate: true })

function onTextChange(value: string) { if (store.selectedBlock) { store.updateBlockText(store.selectedBlock.id, value); emit('changed') } }
function doSplit() { showSplitControl.value = !showSplitControl.value; splitKeyword.value = '' }
function doSplitByKeyword() {
  const kw = splitKeyword.value.trim()
  if (!kw || !store.selectedBlock) return
  const text = store.selectedBlock.current_text
  const positions: number[] = []
  let idx = 0
  while (idx < text.length) { const pos = text.indexOf(kw, idx); if (pos < 0) break; positions.push(pos); idx = pos + kw.length }
  if (positions.length > 0) { emit('split-block', { id: store.selectedBlock.id, positions }) }
  showSplitControl.value = false; splitKeyword.value = ''
}
function addTag() {
  if (!store.selectedBlock || !newTag.value.trim()) return
  emit('add-tag', { id: store.selectedBlock.id, tag: newTag.value.trim() })
  newTag.value = ''
}
</script>

<style scoped>
.block-review-panel { display: flex; flex-direction: column; height: 100%; }
.toolbar { display: flex; justify-content: space-between; align-items: center; padding: 8px 0; border-bottom: 1px solid var(--td-component-stroke); }
.block-count { font-size: 12px; color: var(--td-text-color-secondary); }
.review-body { display: grid; grid-template-columns: 300px 1fr 260px; flex: 1; overflow: hidden; }
.col-list { overflow-y: auto; border-right: 1px solid var(--td-component-stroke); }
.col-editor { overflow-y: auto; padding: 12px 16px; display: flex; flex-direction: column; gap: 10px; }
.col-editor-empty { align-items: center; justify-content: center; }
.col-meta { overflow-y: auto; padding: 12px 16px; background: var(--td-bg-color-container); border-left: 1px solid var(--td-component-stroke); }
.block-item { padding: 10px 12px; border-bottom: 1px solid var(--td-component-stroke); cursor: pointer; transition: background 0.15s; }
.block-item:hover { background: var(--td-bg-color-container-hover); }
.block-item.selected { background: var(--td-bg-color-container-active); }
.block-item.has-error { border-left: 3px solid var(--td-error-color); }
.block-item-header { display: flex; align-items: center; gap: 4px; margin-bottom: 4px; flex-wrap: wrap; }
.block-item-index { font-size: 11px; color: var(--td-text-color-placeholder); }
.block-item-preview { font-size: 12px; color: var(--td-text-color-secondary); line-height: 1.4; }
.meta-section { padding: 12px 0; border-bottom: 1px solid var(--td-component-stroke); }
.meta-section:last-child { border-bottom: none; }
.meta-section h4 { margin: 0 0 8px; font-size: 13px; font-weight: 600; }
.meta-empty { font-size: 12px; color: var(--td-text-color-placeholder); }
.tag-edit-list { display: flex; flex-direction: column; gap: 4px; margin-bottom: 10px; }
.tag-edit-row { display: flex; align-items: center; justify-content: space-between; gap: 4px; padding: 4px 8px; border: 1px solid var(--td-component-stroke); border-radius: 4px; background: var(--td-bg-color-container); }
.tag-edit-text { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; }
.tag-add-row { display: flex; gap: 4px; }
.detail-anomalies { display: flex; flex-direction: column; gap: 4px; }
.anomaly-line { font-size: 12px; line-height: 1.4; padding: 4px 0; display: flex; align-items: flex-start; gap: 4px; }
.anomaly-line.error { color: var(--td-error-color); }
.anomaly-line.warning { color: var(--td-warning-color); }
.anomaly-code { font-size: 10px; font-weight: 600; opacity: .7; }
.detail-empty { flex: 1; display: flex; align-items: center; justify-content: center; }
.split-control { display: flex; align-items: center; gap: 8px; background: var(--td-bg-color-secondarycontainer); padding: 8px; border-radius: 4px; }
.split-hint { font-size: 12px; color: var(--td-text-color-secondary); }
</style>
