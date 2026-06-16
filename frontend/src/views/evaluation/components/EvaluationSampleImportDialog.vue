<template>
  <t-dialog
    v-model:visible="dialogVisible"
    width="760px"
    header="导入样本"
    :confirm-btn="{ content: '导入', loading: saving }"
    @confirm="handleImport"
  >
    <div class="import-body">
      <t-textarea
        v-model="content"
        :autosize="{ minRows: 12, maxRows: 18 }"
        placeholder='粘贴 JSON 数组，或每行一个 JSON 对象的 JSONL'
      />
      <div class="import-hint">支持 JSON 数组和 JSONL；question、reference_answer 必填，reference_contexts 如存在必须为数组且每项包含 text。</div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import type { ReferenceContext } from '@/api/evaluation'

type ImportSample = {
  question: string
  reference_answer: string
  reference_contexts: ReferenceContext[]
}

const props = defineProps<{
  visible: boolean
  saving: boolean
}>()

const emit = defineEmits<{
  (event: 'update:visible', visible: boolean): void
  (event: 'import', samples: ImportSample[]): void
}>()

const dialogVisible = computed({
  get: () => props.visible,
  set: value => emit('update:visible', value),
})

const content = ref('')

function parsePayload(raw: string): Array<{ value: any; label: string }> {
  const trimmed = raw.trim()
  if (!trimmed) throw new Error('请输入要导入的内容')
  if (trimmed.startsWith('[')) {
    let parsed: any
    try {
      parsed = JSON.parse(trimmed)
    } catch (error: any) {
      throw new Error(`JSON 数组解析失败：${error.message || '格式错误'}`)
    }
    if (!Array.isArray(parsed)) throw new Error('JSON 内容必须是数组')
    return parsed.map((value, index) => ({ value, label: `第 ${index + 1} 个样本` }))
  }
  return trimmed
    .split(/\r?\n/)
    .map((line, index) => ({ line: line.trim(), index }))
    .filter(item => item.line)
    .map(item => {
      try {
        return { value: JSON.parse(item.line), label: `第 ${item.index + 1} 行` }
      } catch (error: any) {
        throw new Error(`第 ${item.index + 1} 行 JSON 解析失败：${error.message || '格式错误'}`)
      }
    })
}

function normalizeSample(value: any, label: string): ImportSample {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error(`${label}必须是对象`)
  }
  const question = typeof value.question === 'string' ? value.question.trim() : ''
  const referenceAnswer = typeof value.reference_answer === 'string' ? value.reference_answer.trim() : ''
  if (!question) throw new Error(`${label}缺少 question`)
  if (!referenceAnswer) throw new Error(`${label}缺少 reference_answer`)

  const rawContexts = value.reference_contexts ?? []
  if (!Array.isArray(rawContexts)) {
    throw new Error(`${label}的 reference_contexts 必须是数组`)
  }
  const referenceContexts = rawContexts.map((context: any, index: number) => {
    if (!context || typeof context !== 'object' || Array.isArray(context)) {
      throw new Error(`${label}的第 ${index + 1} 条 reference_contexts 必须是对象`)
    }
    const text = typeof context.text === 'string' ? context.text.trim() : ''
    if (!text) {
      throw new Error(`${label}的第 ${index + 1} 条 reference_contexts 缺少 text`)
    }
    return {
      text,
      ...(typeof context.knowledge_id === 'string' && context.knowledge_id.trim() ? { knowledge_id: context.knowledge_id.trim() } : {}),
      ...(typeof context.chunk_id === 'string' && context.chunk_id.trim() ? { chunk_id: context.chunk_id.trim() } : {}),
    }
  })
  return {
    question,
    reference_answer: referenceAnswer,
    reference_contexts: referenceContexts,
  }
}

function handleImport() {
  try {
    const samples = parsePayload(content.value).map(item => normalizeSample(item.value, item.label))
    if (!samples.length) throw new Error('没有可导入的样本')
    emit('import', samples)
  } catch (error: any) {
    MessagePlugin.warning(error.message || '导入内容格式错误')
  }
}

watch(() => props.visible, visible => {
  if (visible) content.value = ''
})
</script>

<style scoped lang="less">
.import-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.import-hint {
  color: var(--td-text-color-placeholder);
  font-size: 12px;
  line-height: 18px;
}
</style>
