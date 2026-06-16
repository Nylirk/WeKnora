<template>
  <div class="dataset-detail">
    <div class="detail-header">
      <div>
        <div class="breadcrumb">
          <button type="button" class="breadcrumb-link" @click="$emit('back')">评测集</button>
          <span class="breadcrumb-separator">›</span>
          <span class="breadcrumb-current" :title="dataset.name">{{ dataset.name }}</span>
        </div>
        <p class="detail-subtitle">{{ dataset.description || '管理评测样本，用于后续选择知识库创建运行。' }}</p>
      </div>
      <div class="detail-actions">
        <t-button variant="outline" @click="$emit('history')">历史记录</t-button>
        <t-button v-if="canEdit" theme="primary" @click="$emit('create-run')">创建运行</t-button>
      </div>
    </div>

    <div class="sample-toolbar">
      <div>
        <h2>样本预览</h2>
        <span>{{ samples.length }} 条样本</span>
      </div>
      <div v-if="canEdit" class="sample-actions">
        <t-button variant="outline" @click="$emit('import-samples')">导入样本</t-button>
        <t-button theme="primary" @click="$emit('add-sample')">添加样本</t-button>
      </div>
    </div>

    <EvaluationSampleTable
      :samples="samples"
      :loading="loading"
      :can-edit="canEdit"
      @edit="$emit('edit-sample', $event)"
      @delete="$emit('delete-sample', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import type { EvaluationDataset, EvaluationSample } from '@/api/evaluation'
import EvaluationSampleTable from './EvaluationSampleTable.vue'

defineProps<{
  dataset: EvaluationDataset
  samples: EvaluationSample[]
  loading: boolean
  canEdit: boolean
}>()

defineEmits<{
  (event: 'back'): void
  (event: 'history'): void
  (event: 'create-run'): void
  (event: 'add-sample'): void
  (event: 'edit-sample', sample: EvaluationSample): void
  (event: 'delete-sample', sample: EvaluationSample): void
  (event: 'import-samples'): void
}>()
</script>

<style scoped lang="less">
.dataset-detail {
  min-width: 0;
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 18px;
  margin-bottom: 24px;
}

.breadcrumb {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-family: var(--app-font-family);
  font-size: 20px;
  font-weight: 600;
  line-height: 30px;
}

.breadcrumb-link {
  border: none;
  background: transparent;
  padding: 4px 8px;
  margin: -4px -8px;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  font: inherit;
  border-radius: 6px;

  &:hover {
    color: var(--td-success-color);
    background: var(--td-bg-color-container-hover);
  }
}

.breadcrumb-separator {
  color: var(--td-text-color-placeholder);
}

.breadcrumb-current {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--td-text-color-primary);
}

.detail-subtitle {
  margin: 6px 0 0;
  color: var(--td-text-color-placeholder);
  font-size: 14px;
  line-height: 20px;
}

.detail-actions,
.sample-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.sample-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  margin-bottom: 12px;

  h2 {
    margin: 0;
    font-size: 16px;
    line-height: 24px;
  }

  span {
    display: inline-block;
    margin-top: 2px;
    color: var(--td-text-color-placeholder);
    font-size: 12px;
  }
}

@media (max-width: 720px) {
  .detail-header,
  .sample-toolbar {
    flex-direction: column;
  }

  .detail-actions,
  .sample-actions {
    width: 100%;
    justify-content: flex-start;
    flex-wrap: wrap;
  }
}
</style>
