<template>
  <t-dialog
    v-model:visible="dialogVisible"
    width="760px"
    :header="sample ? '编辑评测样本' : '添加评测样本'"
    :confirm-btn="{ content: '保存', loading: saving }"
    @confirm="handleSave"
  >
    <t-form label-align="top" class="sample-form">
      <t-form-item label="问题" required>
        <t-textarea v-model="form.question" :autosize="{ minRows: 2, maxRows: 5 }" />
      </t-form-item>
      <t-form-item label="参考答案" required>
        <t-textarea v-model="form.reference_answer" :autosize="{ minRows: 3, maxRows: 8 }" />
      </t-form-item>
      <t-form-item label="参考上下文">
        <div class="context-editor">
          <div v-for="(context, index) in contexts" :key="index" class="context-item">
            <div class="context-main">
              <t-textarea v-model="context.text" :autosize="{ minRows: 2, maxRows: 6 }" placeholder="上下文文本" />
              <div class="context-meta">
                <t-input v-model="context.knowledge_id" placeholder="knowledge_id（可选）" />
                <t-input v-model="context.chunk_id" placeholder="chunk_id（可选）" />
              </div>
            </div>
            <div class="context-actions">
              <t-button variant="text" shape="square" @click="addContext(index)">
                <template #icon><t-icon name="add" /></template>
              </t-button>
              <t-button variant="text" shape="square" :disabled="contexts.length === 1" @click="removeContext(index)">
                <template #icon><t-icon name="delete" /></template>
              </t-button>
            </div>
          </div>
        </div>
      </t-form-item>
    </t-form>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import type { EvaluationSample, ReferenceContext } from '@/api/evaluation'

type ContextForm = {
  text: string
  knowledge_id: string
  chunk_id: string
}

const props = defineProps<{
  visible: boolean
  sample: EvaluationSample | null
  saving: boolean
}>()

const emit = defineEmits<{
  (event: 'update:visible', visible: boolean): void
  (event: 'save', payload: { question: string; reference_answer: string; reference_contexts: ReferenceContext[] }): void
}>()

const dialogVisible = computed({
  get: () => props.visible,
  set: value => emit('update:visible', value),
})

const form = reactive({ question: '', reference_answer: '' })
const contexts = ref<ContextForm[]>([emptyContext()])

function emptyContext(): ContextForm {
  return { text: '', knowledge_id: '', chunk_id: '' }
}

function resetForm() {
  form.question = props.sample?.question || ''
  form.reference_answer = props.sample?.reference_answer || ''
  const next = props.sample?.reference_contexts?.length
    ? props.sample.reference_contexts.map(context => ({
      text: context.text || '',
      knowledge_id: context.knowledge_id || '',
      chunk_id: context.chunk_id || '',
    }))
    : [emptyContext()]
  contexts.value = next
}

function addContext(index: number) {
  contexts.value.splice(index + 1, 0, emptyContext())
}

function removeContext(index: number) {
  if (contexts.value.length === 1) return
  contexts.value.splice(index, 1)
}

function handleSave() {
  const question = form.question.trim()
  const referenceAnswer = form.reference_answer.trim()
  if (!question || !referenceAnswer) {
    MessagePlugin.warning('问题和参考答案不能为空')
    return
  }
  const referenceContexts = contexts.value
    .map(context => ({
      text: context.text.trim(),
      knowledge_id: context.knowledge_id.trim(),
      chunk_id: context.chunk_id.trim(),
    }))
    .filter(context => context.text)
    .map(context => ({
      text: context.text,
      ...(context.knowledge_id ? { knowledge_id: context.knowledge_id } : {}),
      ...(context.chunk_id ? { chunk_id: context.chunk_id } : {}),
    }))
  emit('save', {
    question,
    reference_answer: referenceAnswer,
    reference_contexts: referenceContexts,
  })
}

watch(() => props.visible, visible => {
  if (visible) resetForm()
})
</script>

<style scoped lang="less">
.sample-form {
  :deep(.t-form__item) {
    margin-bottom: 18px;
  }
}

.context-editor {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.context-item {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}

.context-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.context-meta {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}

.context-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  padding-top: 2px;
}

@media (max-width: 720px) {
  .context-item {
    flex-direction: column;
  }

  .context-actions {
    padding-top: 0;
  }

  .context-meta {
    grid-template-columns: 1fr;
  }
}
</style>
