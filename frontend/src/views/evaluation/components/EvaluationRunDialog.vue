<template>
  <t-dialog
    v-model:visible="dialogVisible"
    width="760px"
    header="创建运行"
    :confirm-btn="{ content: '开始运行', loading: saving }"
    @confirm="handleSave"
  >
    <t-form label-align="top" class="run-form">
      <div class="fixed-dataset">
        <span>当前评测集</span>
        <strong>{{ dataset?.name || '未选择评测集' }}</strong>
      </div>
      <div class="form-grid">
        <t-form-item label="知识库" required>
          <t-select v-model="form.knowledge_base_id" filterable>
            <t-option v-for="kb in knowledgeBases" :key="kb.id" :value="kb.id" :label="kb.name" />
          </t-select>
        </t-form-item>
        <t-form-item label="问答模型" required>
          <t-select v-model="form.chat_model_id" filterable>
            <t-option v-for="model in chatModels" :key="model.id" :value="model.id" :label="model.display_name || model.name" />
          </t-select>
        </t-form-item>
        <t-form-item label="重排模型">
          <t-select v-model="form.rerank_model_id" clearable filterable>
            <t-option v-for="model in rerankModels" :key="model.id" :value="model.id" :label="model.display_name || model.name" />
          </t-select>
        </t-form-item>
      </div>
      <div class="form-grid config-grid">
        <t-form-item label="向量阈值">
          <t-input-number v-model="form.vector_threshold" :min="0" :max="1" :step="0.05" />
        </t-form-item>
        <t-form-item label="关键词阈值">
          <t-input-number v-model="form.keyword_threshold" :min="0" :max="1" :step="0.05" />
        </t-form-item>
        <t-form-item label="召回 Top K">
          <t-input-number v-model="form.embedding_top_k" :min="1" />
        </t-form-item>
        <t-form-item label="重排 Top K">
          <t-input-number v-model="form.rerank_top_k" :min="1" />
        </t-form-item>
        <t-form-item label="重排阈值">
          <t-input-number v-model="form.rerank_threshold" :step="0.05" />
        </t-form-item>
      </div>
      <t-form-item label="评估指标" required>
        <t-checkbox-group v-model="form.metric_names" :options="metricOptions" />
      </t-form-item>
    </t-form>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import type { EvaluationDataset, MetricDefinition } from '@/api/evaluation'
import type { ModelConfig } from '@/api/model'

const props = defineProps<{
  visible: boolean
  dataset: EvaluationDataset | null
  metrics: MetricDefinition[]
  knowledgeBases: any[]
  models: ModelConfig[]
  saving: boolean
}>()

const emit = defineEmits<{
  (event: 'update:visible', visible: boolean): void
  (event: 'save', payload: {
    dataset_id: string
    knowledge_base_id: string
    chat_model_id: string
    rerank_model_id?: string
    vector_threshold: number
    keyword_threshold: number
    embedding_top_k: number
    rerank_top_k: number
    rerank_threshold: number
    metric_names: string[]
  }): void
}>()

const dialogVisible = computed({
  get: () => props.visible,
  set: value => emit('update:visible', value),
})

const form = reactive({
  knowledge_base_id: '',
  chat_model_id: '',
  rerank_model_id: '',
  vector_threshold: 0.15,
  keyword_threshold: 0.3,
  embedding_top_k: 50,
  rerank_top_k: 10,
  rerank_threshold: 0.2,
  metric_names: [] as string[],
})

const chatModels = computed(() => props.models.filter(model => model.type === 'KnowledgeQA'))
const rerankModels = computed(() => props.models.filter(model => model.type === 'Rerank'))
const metricOptions = computed(() => props.metrics.map(metric => ({ label: `${metric.name} (${metric.version})`, value: metric.name })))

function resetForm() {
  form.knowledge_base_id = ''
  form.chat_model_id = chatModels.value[0]?.id || ''
  form.rerank_model_id = ''
  form.vector_threshold = 0.15
  form.keyword_threshold = 0.3
  form.embedding_top_k = 50
  form.rerank_top_k = 10
  form.rerank_threshold = 0.2
  form.metric_names = props.metrics.map(metric => metric.name)
}

function handleSave() {
  if (!props.dataset?.id) {
    MessagePlugin.warning('当前评测集不存在')
    return
  }
  if (!form.knowledge_base_id || !form.chat_model_id) {
    MessagePlugin.warning('请选择知识库和问答模型')
    return
  }
  if (!form.metric_names.length) {
    MessagePlugin.warning('至少选择一个指标')
    return
  }
  emit('save', {
    dataset_id: props.dataset.id,
    knowledge_base_id: form.knowledge_base_id,
    chat_model_id: form.chat_model_id,
    rerank_model_id: form.rerank_model_id || undefined,
    vector_threshold: form.vector_threshold,
    keyword_threshold: form.keyword_threshold,
    embedding_top_k: form.embedding_top_k,
    rerank_top_k: form.rerank_top_k,
    rerank_threshold: form.rerank_threshold,
    metric_names: form.metric_names,
  })
}

watch(() => props.visible, visible => {
  if (visible) resetForm()
})
</script>

<style scoped lang="less">
.fixed-dataset {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 16px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--td-bg-color-secondarycontainer);

  span {
    color: var(--td-text-color-secondary);
    font-size: 13px;
  }

  strong {
    color: var(--td-text-color-primary);
    font-size: 14px;
  }
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0 16px;
}

.config-grid {
  grid-template-columns: repeat(5, minmax(0, 1fr));
}

@media (max-width: 900px) {
  .form-grid,
  .config-grid {
    grid-template-columns: 1fr 1fr;
  }
}
</style>
