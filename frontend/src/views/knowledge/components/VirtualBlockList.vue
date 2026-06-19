<template>
  <RecycleScroller
    class="virtual-block-list col-list"
    :items="items"
    :item-size="itemHeight"
    :buffer="overscan"
    key-field="id"
    :emit-update="false"
  >
    <template #default="{ item }">
      <div
        class="virtual-block-row"
        :class="{
          selected: selectedId === item.id,
          'has-error': anomalyCounts(item).error > 0,
        }"
        @click="emit('select', item.id)"
      >
        <div class="virtual-block-header">
          <t-tag v-if="item.question_number != null" size="small" theme="primary" variant="light">#{{ item.question_number }}</t-tag>
          <t-tag v-else size="small" theme="default">无题号</t-tag>
          <span class="virtual-block-index">idx {{ idxOf(item) }}</span>
          <t-space size="2px">
            <t-tag v-if="anomalyCounts(item).error > 0" size="small" theme="danger" variant="light">
              {{ anomalyCounts(item).error }} errors
            </t-tag>
            <t-tag v-if="anomalyCounts(item).warning > 0" size="small" theme="warning" variant="light">
              {{ anomalyCounts(item).warning }} warnings
            </t-tag>
          </t-space>
        </div>
        <div class="virtual-block-preview">{{ preview(item) }}</div>
      </div>
    </template>
  </RecycleScroller>
  <t-empty v-if="items.length === 0" description="无 blocks" />
</template>

<script setup lang="ts">
import { RecycleScroller } from 'vue-virtual-scroller'
import 'vue-virtual-scroller/dist/vue-virtual-scroller.css'
import type { ImportBlock, ImportBlockAnomaly } from '@/api/question_block'
import { normalizeAnomalySeverity } from '@/stores/importWorkbench'

const props = withDefaults(defineProps<{
  items: ImportBlock[]
  itemHeight?: number
  overscan?: number
  selectedId: string | null
  getAnomalies: (blockId: string) => ImportBlockAnomaly[]
}>(), {
  itemHeight: 76,
  overscan: 10,
})

const emit = defineEmits<{ select: [id: string] }>()

function preview(block: ImportBlock): string {
  const text = block.current_text || ''
  return text.length > 100 ? text.slice(0, 100) + '…' : text
}

function idxOf(block: ImportBlock): number {
  return props.items.findIndex(b => b.id === block.id)
}

function anomalyCounts(block: ImportBlock) {
  const anomalies = props.getAnomalies(block.id)
  let error = 0; let warning = 0
  for (const a of anomalies) {
    const sev = normalizeAnomalySeverity(a)
    if (sev === 'error') error++
    else warning++
  }
  return { error, warning }
}
</script>

<style scoped>
.virtual-block-list {
  height: 100%;
  overflow-y: auto;
  border-right: 1px solid var(--td-component-stroke);
}

.virtual-block-row {
  padding: 10px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  cursor: pointer;
  transition: background 0.15s;
  display: flex;
  flex-direction: column;
  justify-content: center;
  height: 76px;
  box-sizing: border-box;
}

.virtual-block-row:hover { background: var(--td-bg-color-container-hover); }
.virtual-block-row.selected { background: var(--td-bg-color-container-active); }
.virtual-block-row.has-error { border-left: 3px solid var(--td-error-color); }

.virtual-block-header {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 4px;
  flex-wrap: wrap;
}

.virtual-block-index {
  font-size: 11px;
  color: var(--td-text-color-placeholder);
}

.virtual-block-preview {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  line-height: 1.4;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
