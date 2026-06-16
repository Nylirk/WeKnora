<template>
  <div class="sample-table-wrap">
    <t-loading :loading="loading" size="small">
      <table class="sample-table">
        <thead>
          <tr>
            <th class="question-col">question</th>
            <th class="answer-col">reference_answer</th>
            <th class="contexts-col">reference_contexts</th>
            <th class="operation-col">操作</th>
          </tr>
        </thead>
        <tbody v-if="samples.length > 0">
          <tr v-for="sample in samples" :key="sample.id">
              <td><div class="cell-text">{{ sample.question }}</div></td>
              <td><div class="cell-text">{{ sample.reference_answer }}</div></td>
              <td>
                <t-popup
                  v-if="sample.reference_contexts?.length"
                  trigger="click"
                  placement="bottom"
                  destroy-on-close
                  overlayClassName="sample-context-popup"
                >
                <button
                  type="button"
                  class="contexts-toggle"
                >
                  {{ sample.reference_contexts?.length || 0 }} 条上下文
                  <t-icon name="chevron-down" size="13px" />
                </button>
                <template #content>
                  <div class="contexts-popover">
                    <div
                      v-for="(context, index) in sample.reference_contexts"
                      :key="`${sample.id}-${index}`"
                      class="context-item"
                    >
                      <p>{{ context.text }}</p>
                      <div v-if="context.knowledge_id || context.chunk_id" class="context-meta">
                        <span v-if="context.knowledge_id">knowledge_id: {{ context.knowledge_id }}</span>
                        <span v-if="context.chunk_id">chunk_id: {{ context.chunk_id }}</span>
                      </div>
                    </div>
                  </div>
                </template>
                </t-popup>
                <button v-else type="button" class="contexts-toggle" disabled>
                  0 条上下文
                </button>
              </td>
              <td>
                <t-space v-if="canEdit" size="small">
                  <t-link theme="primary" @click="$emit('edit', sample)">编辑</t-link>
                  <t-popconfirm content="确认删除该样本？" @confirm="$emit('delete', sample)">
                    <t-link theme="danger">删除</t-link>
                  </t-popconfirm>
                </t-space>
              </td>
          </tr>
        </tbody>
      </table>
      <div v-if="!loading && samples.length === 0" class="empty-state">
        <t-empty description="暂无样本" />
      </div>
    </t-loading>
  </div>
</template>

<script setup lang="ts">
import type { EvaluationSample } from '@/api/evaluation'

defineProps<{
  samples: EvaluationSample[]
  loading: boolean
  canEdit: boolean
}>()

defineEmits<{
  (event: 'edit', sample: EvaluationSample): void
  (event: 'delete', sample: EvaluationSample): void
}>()

</script>

<style scoped lang="less">
.sample-table-wrap {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  overflow: hidden;
  background: var(--td-bg-color-container);
}

.sample-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
  font-size: 13px;

  th {
    height: 40px;
    padding: 0 14px;
    border-bottom: 1px solid var(--td-component-stroke);
    border-right: 1px solid var(--td-component-stroke);
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-text-color-secondary);
    font-weight: 600;
    text-align: left;

    &:last-child {
      border-right: none;
    }
  }

  td {
    height: 48px;
    padding: 10px 14px;
    border-bottom: 1px solid var(--td-component-stroke);
    border-right: 1px solid var(--td-component-stroke);
    color: var(--td-text-color-primary);
    vertical-align: top;

    &:last-child {
      border-right: none;
    }
  }

  tbody tr:hover td {
    background: var(--td-bg-color-container-hover);
  }

  tr:last-child td {
    border-bottom: none;
  }
}

.question-col {
  width: 32%;
}

.answer-col {
  width: 38%;
}

.contexts-col {
  width: 16%;
}

.operation-col {
  width: 120px;
}

.cell-text {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  line-clamp: 3;
  overflow: hidden;
  line-height: 20px;
  white-space: pre-wrap;
}

.contexts-toggle {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: none;
  background: transparent;
  padding: 0;
  color: var(--td-brand-color);
  cursor: pointer;
  font-size: 13px;

  &:disabled {
    color: var(--td-text-color-placeholder);
    cursor: default;
  }
}

.contexts-popover {
  width: min(520px, calc(100vw - 48px));
  max-height: 360px;
  overflow-y: auto;
  padding: 4px 0;
  background: var(--td-bg-color-container);
}

.context-item {
  padding: 10px 12px;
  border-bottom: 1px solid var(--td-component-stroke);

  &:last-child {
    border-bottom: none;
  }

  p {
    margin: 0;
    color: var(--td-text-color-primary);
    line-height: 20px;
    white-space: pre-wrap;
  }
}

.context-meta {
  display: flex;
  gap: 12px;
  margin-top: 4px;
  color: var(--td-text-color-placeholder);
  font-size: 12px;
  flex-wrap: wrap;
}

.empty-state {
  min-height: 240px;
  display: flex;
  align-items: center;
  justify-content: center;
}

@media (max-width: 900px) {
  .sample-table {
    min-width: 860px;
  }

  .sample-table-wrap {
    overflow-x: auto;
  }
}
</style>
