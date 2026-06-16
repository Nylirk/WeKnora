<template>
  <section class="dataset-section">
    <div class="section-header" role="button" tabindex="0" @click="$emit('toggle')" @keydown.enter.prevent="$emit('toggle')">
      <t-icon name="user" size="14px" />
      <span>我创建的</span>
      <span class="section-count">{{ datasets.length }}</span>
      <t-icon class="section-toggle" :name="collapsed ? 'chevron-right' : 'chevron-down'" size="14px" />
    </div>

    <div v-if="loading && datasets.length === 0" class="dataset-card-grid">
      <div v-for="item in 6" :key="item" class="dataset-card-skeleton">
        <t-skeleton animation="gradient" :row-col="[{ width: '62%', height: '20px' }, { width: '100%', height: '14px' }, { width: '78%', height: '14px' }]" />
      </div>
    </div>

    <div v-else-if="!collapsed && datasets.length > 0" class="dataset-card-grid">
      <EvaluationDatasetCard
        v-for="dataset in datasets"
        :key="dataset.id"
        :dataset="dataset"
        :can-edit="canEdit"
        @open="$emit('open', $event)"
        @edit="$emit('edit', $event)"
        @delete="$emit('delete', $event)"
      />
    </div>

    <div v-else-if="!collapsed" class="empty-state">
      <t-empty description="暂无评测集" />
    </div>
  </section>
</template>

<script setup lang="ts">
import type { EvaluationDataset } from '@/api/evaluation'
import EvaluationDatasetCard from './EvaluationDatasetCard.vue'

defineProps<{
  datasets: EvaluationDataset[]
  loading: boolean
  canEdit: boolean
  collapsed: boolean
}>()

defineEmits<{
  (event: 'toggle'): void
  (event: 'open', dataset: EvaluationDataset): void
  (event: 'edit', dataset: EvaluationDataset): void
  (event: 'delete', dataset: EvaluationDataset): void
}>()
</script>

<style scoped lang="less">
.dataset-section {
  min-width: 0;
}

.section-header {
  grid-column: 1 / -1;
  display: flex;
  align-items: center;
  gap: 6px;
  position: sticky;
  top: 0;
  z-index: 5;
  background: var(--td-bg-color-container);
  box-shadow: 0 -8px 0 0 var(--td-bg-color-container), 0 4px 0 0 var(--td-bg-color-container);
  padding: 6px 4px 8px 0;
  color: var(--td-text-color-secondary);
  font-family: var(--app-font-family);
  font-size: 13px;
  font-weight: 600;
  line-height: 20px;
  cursor: pointer;
  user-select: none;
  outline: none;

  &:hover,
  &:focus-visible {
    color: var(--td-text-color-primary);
  }
}

.section-count {
  margin-left: 2px;
  padding: 0 6px;
  border-radius: 8px;
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
  font-size: 11px;
  line-height: 16px;
  font-weight: 500;
}

.section-toggle {
  margin-left: 4px;
  opacity: 0.7;
}

.dataset-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 320px));
  justify-content: start;
  gap: 16px;
  margin-top: 16px;
  animation: contentFadeIn 0.32s ease-out;
}

.dataset-card-skeleton {
  height: 120px;
  min-height: 120px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  padding: 14px;
  box-sizing: border-box;
  background: var(--td-bg-color-container);
}

.empty-state {
  min-height: 260px;
  margin-top: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
}

@keyframes contentFadeIn {
  from {
    opacity: 0;
    transform: translateY(6px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (max-width: 640px) {
  .dataset-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>
