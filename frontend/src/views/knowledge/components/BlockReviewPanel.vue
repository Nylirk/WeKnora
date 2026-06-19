<template>
  <div class="block-review-panel">
    <div class="toolbar">
      <t-space>
        <t-select v-model="store.anomalyFilter" size="small" style="width: 120px">
          <t-option value="all" label="全部" />
          <t-option value="error" label="错误" />
          <t-option value="warning" label="警告" />
        </t-select>
        <t-button size="small" variant="outline" @click="handleSort">按题号排序</t-button>
        <t-button size="small" variant="outline" theme="warning" :disabled="!store.hasDeletedBlocks" @click="restoreAllDeleted">
          恢复删除
        </t-button>
      </t-space>
      <span class="block-count">{{ store.filteredBlocks.length }} / {{ store.blocks.length }} blocks</span>
    </div>

    <div class="review-body">
      <div class="block-list">
        <div
          v-for="block in store.filteredBlocks"
          :key="block.id"
          class="block-item"
          :class="{ selected: store.selectedBlockId === block.id, 'has-error': block.anomalies.some(a => a.severity === 'error') }"
          @click="store.selectBlock(block.id)"
        >
          <div class="block-item-header">
            <t-tag v-if="block.question_number != null" size="small" theme="primary" variant="light">#{{ block.question_number }}</t-tag>
            <t-tag v-else size="small" theme="default">无题号</t-tag>
            <span class="block-item-index">idx {{ block.index }}</span>
            <t-space size="2px">
              <span v-for="a in block.anomalies" :key="a.code">
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

      <div class="block-detail" v-if="store.selectedBlock">
        <div class="detail-toolbar">
          <t-space size="small">
            <t-button size="small" variant="outline" @click="store.restoreOriginalText(store.selectedBlock!.id); emit('changed')">
              恢复原始文本
            </t-button>
            <t-button size="small" variant="outline" @click="doSplit">拆分</t-button>
            <t-button size="small" variant="outline" @click="store.mergeWithPrevious(store.selectedBlock!.id); emit('changed')">合并上一个</t-button>
            <t-button size="small" variant="outline" @click="store.mergeWithNext(store.selectedBlock!.id); emit('changed')">合并下一个</t-button>
            <t-button size="small" variant="outline" theme="danger" @click="store.deleteBlock(store.selectedBlock!.id); emit('changed')">删除</t-button>
          </t-space>
        </div>
        <div class="detail-meta">
          <t-space size="small" v-if="store.selectedBlock.tags.length">
            <t-tag v-for="(tag, i) in store.selectedBlock.tags" :key="i" size="small" variant="outline">{{ tag }}</t-tag>
          </t-space>
          <div class="detail-anomalies" v-if="store.selectedBlock.anomalies.length">
            <div v-for="a in store.selectedBlock.anomalies" :key="a.code" class="anomaly-line" :class="a.severity">
              <span class="anomaly-severity" :class="a.severity">{{ a.severity === 'error' ? '❌' : '⚠️' }}</span>
              {{ a.message }}
            </div>
          </div>
        </div>
        <t-textarea
          v-model="editingText"
          :autosize="{ minRows: 6, maxRows: 20 }"
          @change="onTextChange"
        />
        <div class="split-control" v-if="showSplitControl">
          <span class="split-hint">输入拆分关键词（如 "249" 按题号拆）：</span>
          <t-input v-model="splitKeyword" size="small" style="width: 120px" placeholder="如: 249" @enter="doSplitByKeyword" />
          <t-button size="small" @click="doSplitByKeyword">执行拆分</t-button>
        </div>
      </div>
      <t-empty v-else description="选择一个 block 查看详情" class="detail-empty" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useImportWorkbenchStore } from '@/stores/importWorkbench'

const store = useImportWorkbenchStore()
const emit = defineEmits<{ changed: [] }>()

const editingText = ref('')
const showSplitControl = ref(false)
const splitKeyword = ref('')

watch(() => store.selectedBlock, (block) => {
  editingText.value = block?.current_text ?? ''
  showSplitControl.value = false
  splitKeyword.value = ''
}, { immediate: true })

function onTextChange(value: string) {
  if (store.selectedBlock) {
    store.updateBlockText(store.selectedBlock.id, value)
    emit('changed')
  }
}

function handleSort() {
  store.sortBlocksByQuestionNumber()
  emit('changed')
}

function doSplit() {
  showSplitControl.value = !showSplitControl.value
  splitKeyword.value = ''
}

function doSplitByKeyword() {
  const kw = splitKeyword.value.trim()
  if (!kw || !store.selectedBlock) return
  const text = store.selectedBlock.current_text
  const positions: number[] = []
  let idx = 0
  while (idx < text.length) {
    const pos = text.indexOf(kw, idx)
    if (pos < 0) break
    positions.push(pos)
    idx = pos + kw.length
  }
  if (positions.length > 0) {
    store.splitBlock(store.selectedBlock.id, positions)
    emit('changed')
  }
  showSplitControl.value = false
  splitKeyword.value = ''
}

function restoreAllDeleted() {
  while (store.deletedBlocks.length > 0) {
    const b = store.deletedBlocks[store.deletedBlocks.length - 1]
    store.restoreBlock(b.id)
  }
  emit('changed')
}
</script>

<style scoped>
.block-review-panel { display: flex; flex-direction: column; height: 100%; }
.toolbar { display: flex; justify-content: space-between; align-items: center; padding: 8px 0; border-bottom: 1px solid var(--td-component-stroke); }
.block-count { font-size: 12px; color: var(--td-text-color-secondary); }
.review-body { display: flex; flex: 1; overflow: hidden; }
.block-list { width: 320px; flex-shrink: 0; overflow-y: auto; border-right: 1px solid var(--td-component-stroke); }
.block-item { padding: 10px 12px; border-bottom: 1px solid var(--td-component-stroke); cursor: pointer; transition: background 0.15s; }
.block-item:hover { background: var(--td-bg-color-container-hover); }
.block-item.selected { background: var(--td-bg-color-container-active); }
.block-item.has-error { border-left: 3px solid var(--td-error-color); }
.block-item-header { display: flex; align-items: center; gap: 4px; margin-bottom: 4px; flex-wrap: wrap; }
.block-item-index { font-size: 11px; color: var(--td-text-color-placeholder); }
.block-item-preview { font-size: 12px; color: var(--td-text-color-secondary); line-height: 1.4; }
.block-detail { flex: 1; padding: 12px 16px; overflow-y: auto; display: flex; flex-direction: column; gap: 10px; }
.detail-toolbar { }
.detail-meta { display: flex; flex-direction: column; gap: 6px; }
.detail-anomalies { }
.anomaly-line { font-size: 12px; padding: 2px 0; }
.anomaly-line.error { color: var(--td-error-color); }
.anomaly-line.warning { color: var(--td-warning-color); }
.anomaly-severity { margin-right: 4px; }
.detail-empty { flex: 1; display: flex; align-items: center; justify-content: center; }
.split-control { display: flex; align-items: center; gap: 8px; background: var(--td-bg-color-secondarycontainer); padding: 8px; border-radius: 4px; }
.split-hint { font-size: 12px; color: var(--td-text-color-secondary); }
</style>
