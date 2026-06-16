<template>
  <div class="evaluation-dataset-card" role="button" tabindex="0" @click="$emit('open', dataset)" @keydown.enter.prevent="$emit('open', dataset)">
    <div class="card-header">
      <span class="card-title" :title="dataset.name">{{ dataset.name }}</span>
      <t-popup v-if="canEdit" overlayClassName="card-more-popup" trigger="click" destroy-on-close placement="bottom-right">
        <div class="more-wrap" @click.stop>
          <img class="more-icon" src="@/assets/img/more.png" alt="" />
        </div>
        <template #content>
          <div class="popup-menu" @click.stop>
            <div class="popup-menu-item" @click.stop="$emit('edit', dataset)">
              <t-icon class="menu-icon" name="edit-1" />
              <span>编辑</span>
            </div>
            <t-popconfirm content="删除评测集后不可继续编辑，历史运行不受影响。" @confirm="$emit('delete', dataset)">
              <div class="popup-menu-item delete" @click.stop>
                <t-icon class="menu-icon" name="delete" />
                <span>删除</span>
              </div>
            </t-popconfirm>
          </div>
        </template>
      </t-popup>
    </div>
    <div class="card-content">
      <div class="card-description">{{ dataset.description || '无描述' }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { EvaluationDataset } from '@/api/evaluation'

defineProps<{
  dataset: EvaluationDataset
  canEdit: boolean
}>()

defineEmits<{
  (event: 'open', dataset: EvaluationDataset): void
  (event: 'edit', dataset: EvaluationDataset): void
  (event: 'delete', dataset: EvaluationDataset): void
}>()
</script>

<style scoped lang="less">
.evaluation-dataset-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  overflow: hidden;
  box-sizing: border-box;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  background: linear-gradient(135deg, var(--td-bg-color-container) 0%, rgba(7, 192, 95, 0.04) 100%);
  position: relative;
  cursor: pointer;
  transition: border-color 0.2s ease, box-shadow 0.2s ease, background 0.2s ease;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  height: 136px;
  min-height: 136px;
  outline: none;

  &::after {
    content: '';
    position: absolute;
    top: 0;
    right: 0;
    width: 60px;
    height: 60px;
    background: linear-gradient(135deg, rgba(7, 192, 95, 0.08) 0%, transparent 100%);
    border-radius: 0 8px 0 100%;
    pointer-events: none;
    z-index: 0;
  }

  &:hover,
  &:focus-visible {
    border-color: var(--td-brand-color);
    box-shadow: 0 4px 12px rgba(7, 192, 95, 0.12);
    background: linear-gradient(135deg, var(--td-bg-color-container) 0%, rgba(7, 192, 95, 0.08) 100%);
  }

  .card-header,
  .card-content {
    position: relative;
    z-index: 1;
  }
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 4px;
  margin-bottom: 8px;
}

.card-title {
  flex: 1;
  min-width: 0;
  color: var(--td-text-color-primary);
  font-family: var(--app-font-family);
  font-size: 15px;
  font-weight: 600;
  line-height: 22px;
  letter-spacing: 0.01em;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.more-wrap {
  display: flex;
  width: 24px;
  height: 24px;
  justify-content: center;
  align-items: center;
  border-radius: 6px;
  cursor: pointer;
  flex-shrink: 0;
  transition: all 0.2s ease;
  opacity: 0;

  .evaluation-dataset-card:hover &,
  .evaluation-dataset-card:focus-visible & {
    opacity: 0.6;
  }

  &:hover {
    background: var(--td-bg-color-container-hover);
    opacity: 1;
  }
}

.more-icon {
  width: 14px;
  height: 14px;
}

.card-content {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.card-description {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  line-clamp: 3;
  overflow: hidden;
  color: var(--td-text-color-secondary);
  font-family: var(--app-font-family);
  font-size: 12px;
  font-weight: 400;
  line-height: 18px;
}

.popup-menu {
  display: flex;
  flex-direction: column;
  min-width: 128px;
  padding: 4px;
}

.popup-menu-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  cursor: pointer;
  color: var(--td-text-color-primary);
  transition: all 0.15s ease;
  border-radius: 6px;
  font-size: 14px;
  line-height: 20px;

  &:hover {
    background: var(--td-bg-color-container-hover);
  }

  .menu-icon {
    font-size: 16px;
    color: var(--td-text-color-secondary);
  }

  &.delete {
    color: var(--td-error-color-6);
    margin-top: 4px;

    .menu-icon {
      color: var(--td-error-color-6);
    }

    &:hover {
      background: var(--td-error-color-1);
    }
  }
}
</style>
