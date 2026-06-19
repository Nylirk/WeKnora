<template>
  <div class="block-review-panel">
    <div class="toolbar">
      <t-space>
        <t-select v-model="store.anomalyFilter" size="small" style="width: 120px">
          <t-option value="all" label="全部" />
          <t-option value="error" label="错误" />
          <t-option value="warning" label="警告" />
        </t-select>
        <t-button size="small" variant="outline" :disabled="importUI.blocking" @click="emit('sort')">按题号排序</t-button>
        <t-button size="small" variant="outline" theme="warning" :disabled="!store.hasDeletedBlocks || importUI.blocking" @click="restoreDialogVisible = true">恢复删除</t-button>
      </t-space>
      <span class="block-count">{{ store.filteredBlocks.length }} / {{ store.blockOrder.length }} blocks</span>
    </div>

    <div class="review-body">
      <VirtualBlockList
        :items="store.filteredBlocks"
        :selected-id="store.selectedBlockId"
        :get-anomalies="(id: string) => store.getMergedAnomalies(id)"
        @select="store.selectBlock"
      />

      <div class="col-editor" v-if="store.selectedBlock">
        <div class="detail-toolbar">
          <t-space size="small">
            <t-tooltip :content="selectedBlockDirty ? '' : '当前文本未修改，无需恢复'">
              <t-button size="small" variant="outline" :disabled="importUI.blocking || !selectedBlockDirty" @click="emit('restore-original', store.selectedBlock!.id)">恢复原始文本</t-button>
            </t-tooltip>
            <t-button size="small" variant="outline" :disabled="importUI.blocking" @click="doSplit">拆分</t-button>
            <t-button size="small" variant="outline" :disabled="importUI.blocking" @click="emit('merge-previous', store.selectedBlock!.id)">合并上一个</t-button>
            <t-button size="small" variant="outline" :disabled="importUI.blocking" @click="emit('merge-next', store.selectedBlock!.id)">合并下一个</t-button>
            <t-button size="small" variant="outline" theme="danger" :disabled="importUI.blocking" @click="emit('delete-block', store.selectedBlock!.id)">删除</t-button>
          </t-space>
        </div>
        <t-textarea v-model="editingText" :autosize="{ minRows: 6, maxRows: 20 }" @change="onTextChange" />
        <div class="split-control" v-if="showSplitControl">
          <span class="split-hint">输入拆分关键词（如 "249"）：</span>
          <t-input v-model="splitKeyword" size="small" style="width: 120px" placeholder="如: 249" @enter="doSplitByKeyword" />
          <t-button size="small" :disabled="importUI.blocking" @click="doSplitByKeyword">执行拆分</t-button>
        </div>
      </div>
      <t-empty v-else description="选择一个 block" class="col-editor col-editor-empty" />

      <aside class="col-meta" v-if="store.selectedBlock">
        <section class="meta-section">
          <h4>标签</h4>
          <div class="tag-cloud">
            <button
              v-for="tag in selectedBlockTags"
              :key="tag"
              type="button"
              class="tag-pill"
              @mouseenter="hoveredTag = tag"
              @mouseleave="hoveredTag = ''"
            >
              <span>{{ tag }}</span>
              <span
                v-show="hoveredTag === tag"
                class="tag-remove"
                @click.stop="emit('remove-tag', { id: store.selectedBlock!.id, tag })"
              >×</span>
            </button>

            <template v-if="addingTag">
              <input
                ref="tagInputRef"
                v-model="newTag"
                class="tag-add-input"
                placeholder="添加标签"
                @keydown.enter.prevent="confirmAddTag"
                @keydown.esc.prevent="cancelAddTag"
                @blur="cancelAddTag"
              />
            </template>

            <button
              v-else
              type="button"
              class="tag-pill tag-add-pill"
              :disabled="importUI.blocking"
              @click="startAddTag"
            >
              + 添加标签
            </button>
          </div>
        </section>
        <section class="meta-section">
          <h4>异常信息</h4>
          <div v-if="selectedBlockAnomalies.length" class="detail-anomalies">
            <div v-for="a in selectedBlockAnomalies" :key="a.code + ':' + a.message" class="anomaly-card" :class="normalizeAnomalySeverity(a)">
              <div class="anomaly-code">{{ a.code }}</div>
              <div class="anomaly-message">{{ a.message }}</div>
            </div>
          </div>
          <span v-else class="meta-empty">无异常</span>
        </section>
      </aside>
    </div>
  </div>

  <!-- P4: restore deleted block dialog -->
  <t-dialog
    v-model:visible="restoreDialogVisible"
    header="恢复删除的分块"
    :z-index="4000"
    attach="body"
    :footer="false"
    width="480px"
  >
    <div v-if="deletedBlockList.length === 0" class="restore-empty">
      <t-empty description="没有已删除的分块" />
    </div>
    <div v-else class="deleted-block-list">
      <div v-for="block in deletedBlockList" :key="block.id" class="deleted-block-row">
        <div class="deleted-block-info">
          <div class="deleted-block-title">
            <t-tag v-if="block.question_number != null" size="small" theme="primary" variant="light">#{{ block.question_number }}</t-tag>
            <t-tag v-else size="small" theme="default">无题号</t-tag>
            <span class="deleted-block-idx">idx {{ block.index }}</span>
            <t-tag v-for="a in (store.getMergedAnomalies(block.id).slice(0, 2))" :key="a.code" size="small" :theme="a.severity === 'error' ? 'danger' : 'warning'" variant="light">{{ a.code }}</t-tag>
          </div>
          <p class="deleted-block-preview">{{ (block.current_text || '').slice(0, 80) }}{{ (block.current_text || '').length > 80 ? '…' : '' }}</p>
        </div>
        <t-button size="small" variant="outline" theme="primary" :disabled="importUI.blocking" @click="doRestoreDeleted(block.id)">恢复</t-button>
      </div>
    </div>
    <template #footer v-if="deletedBlockList.length > 0">
      <t-button variant="outline" @click="restoreDialogVisible = false">关闭</t-button>
    </template>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { useImportWorkbenchStore, normalizeAnomalySeverity } from '@/stores/importWorkbench'
import { useImportUIStore } from '@/stores/importUIStore'
import VirtualBlockList from './VirtualBlockList.vue'

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
  'restore-deleted': [id: string]
  'add-tag': [payload: { id: string; tag: string }]
  'remove-tag': [payload: { id: string; tag: string }]
}>()

const editingText = ref('')
const showSplitControl = ref(false)
const splitKeyword = ref('')

// P3-P5: tag pill cloud
const newTag = ref('')
const addingTag = ref(false)
const hoveredTag = ref('')
const tagInputRef = ref<HTMLInputElement | null>(null)

const restoreDialogVisible = ref(false)

const selectedBlockAnomalies = computed(() =>
  store.selectedBlock ? store.getMergedAnomalies(store.selectedBlock.id) : []
)
const selectedBlockTags = computed(() =>
  Array.isArray(store.selectedBlock?.tags) ? store.selectedBlock.tags : []
)

const selectedBlockDirty = computed(() =>
  store.selectedBlock &&
  store.selectedBlock.current_text !== store.selectedBlock.original_text
)

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
// P3-P5: tag pill cloud
function startAddTag() {
  addingTag.value = true
  newTag.value = ''
  nextTick(() => tagInputRef.value?.focus())
}
function confirmAddTag() {
  const tag = newTag.value.trim()
  if (!tag) { cancelAddTag(); return }
  emit('add-tag', { id: store.selectedBlock!.id, tag })
  addingTag.value = false
  newTag.value = ''
}
function cancelAddTag() {
  addingTag.value = false
  newTag.value = ''
}

// P4: restore deleted
const deletedBlockList = computed(() =>
  store.deletedBlockStack.map(id => store.deletedBlockMap[id]).filter(Boolean)
)
function doRestoreDeleted(id: string) {
  restoreDialogVisible.value = false
  emit('restore-deleted', id)
}
</script>

<style scoped>
.block-review-panel { display: flex; flex-direction: column; height: 100%; }
.toolbar { display: flex; justify-content: space-between; align-items: center; padding: 8px 0; border-bottom: 1px solid var(--td-component-stroke); }
.block-count { font-size: 12px; color: var(--td-text-color-secondary); }
.review-body { display: grid; grid-template-columns: 300px 1fr 260px; flex: 1; overflow: hidden; }
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

/* P3-P5: tag pill cloud */
.tag-cloud {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 12px;
  align-items: center;
}
.tag-pill {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  min-height: 30px;
  padding: 0 12px;
  border: none;
  border-radius: 999px;
  background: #2563eb;
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  line-height: 30px;
  cursor: default;
  white-space: nowrap;
}
.tag-pill:hover { background: #1d4ed8; }
.tag-remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 999px;
  font-size: 13px;
  line-height: 16px;
  cursor: pointer;
  color: #fff;
  opacity: 0.9;
}
.tag-remove:hover { background: rgba(255, 255, 255, 0.22); }
.tag-add-pill {
  background: var(--td-bg-color-container);
  color: var(--td-brand-color);
  border: 1px dashed var(--td-brand-color);
  cursor: pointer;
}
.tag-add-pill:hover { background: var(--td-brand-color-light); }
.tag-add-input {
  height: 30px;
  min-width: 120px;
  max-width: 180px;
  padding: 0 10px;
  border: 1px solid var(--td-brand-color);
  border-radius: 999px;
  outline: none;
  font-size: 13px;
}

/* P1: anomaly cards — error/warning only, no green */
.detail-anomalies { display: flex; flex-direction: column; gap: 6px; }
.anomaly-card {
  padding: 9px 10px;
  border-radius: 9px;
  border: 1px solid;
}
.anomaly-card.error {
  border-color: var(--td-error-color-5);
  background: var(--td-error-color-1);
}
.anomaly-card.warning {
  border-color: var(--td-warning-color-5);
  background: var(--td-warning-color-1);
}
.anomaly-card .anomaly-code {
  font-size: 11px;
  font-weight: 600;
  line-height: 16px;
  color: var(--td-text-color-secondary);
  word-break: break-all;
}
.anomaly-card .anomaly-message {
  margin-top: 3px;
  font-size: 12px;
  line-height: 18px;
  color: var(--td-text-color-primary);
}
.split-control { display: flex; align-items: center; gap: 8px; background: var(--td-bg-color-secondarycontainer); padding: 8px; border-radius: 4px; }
.split-hint { font-size: 12px; color: var(--td-text-color-secondary); }

/* P4: restore deleted dialog */
.restore-empty { padding: 24px 0; }
.deleted-block-list { display: flex; flex-direction: column; gap: 8px; max-height: 50vh; overflow-y: auto; }
.deleted-block-row {
  display: flex; align-items: center; gap: 12px;
  padding: 10px 12px; border: 1px solid var(--td-component-stroke);
  border-radius: 8px; background: var(--td-bg-color-container);
}
.deleted-block-info { flex: 1; min-width: 0; }
.deleted-block-title { display: flex; align-items: center; gap: 4px; margin-bottom: 4px; flex-wrap: wrap; }
.deleted-block-idx { font-size: 11px; color: var(--td-text-color-placeholder); }
.deleted-block-preview {
  margin: 0; font-size: 12px; color: var(--td-text-color-secondary);
  line-height: 1.4; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
</style>
