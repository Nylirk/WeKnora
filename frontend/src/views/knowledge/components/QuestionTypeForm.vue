<template>
  <div class="question-type-form">
    <template v-if="isChoiceType">
      <div v-for="(opt, i) in options" :key="i" class="option-row">
        <t-input v-model="opt.label" :placeholder="'标签 (A,B,C...)'" style="width: 80px" />
        <t-input v-model="opt.content" :placeholder="'选项内容'" style="flex: 1" />
        <t-button variant="text" theme="danger" @click="options.splice(i, 1)">
          <template #icon><t-icon name="close" /></template>
        </t-button>
      </div>
      <t-button variant="dashed" @click="options.push({ label: '', content: '' })">
        <template #icon><t-icon name="add" /></template>
        添加选项
      </t-button>
    </template>
    <template v-else-if="questionType === 'fill_blank'">
      <div v-for="(_, i) in blankAnswers" :key="i" class="option-row">
        <t-input v-model="blankAnswers[i]" :placeholder="'填空答案'" style="flex: 1" />
        <t-button variant="text" theme="danger" @click="blankAnswers.splice(i, 1)">
          <template #icon><t-icon name="close" /></template>
        </t-button>
      </div>
      <t-button variant="dashed" @click="blankAnswers.push('')">
        <template #icon><t-icon name="add" /></template>
        添加填空
      </t-button>
    </template>
    <template v-else-if="questionType === 'true_false'">
      <t-radio-group v-model="trueFalseValue">
        <t-radio :value="true">正确</t-radio>
        <t-radio :value="false">错误</t-radio>
      </t-radio-group>
    </template>
    <template v-else-if="questionType === 'composite'">
      <QuestionJsonEditor v-model="internalValue" />
    </template>
    <template v-else>
      <QuestionJsonEditor v-model="internalValue" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import type { QuestionType } from '@/api/question'

const props = defineProps<{ questionType: QuestionType; modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [v: string] }>()

const options = ref<Array<{ label: string; content: string }>>([])
const blankAnswers = ref<string[]>([])
const trueFalseValue = ref(true)
const internalValue = ref(props.modelValue || '{}')

const isChoiceType = computed(() => props.questionType === 'single_choice' || props.questionType === 'multiple_choice')

watch(() => props.modelValue, (v) => {
  if (!v) return
  try {
    const parsed = JSON.parse(v)
    if (isChoiceType.value && Array.isArray(parsed)) {
      options.value = parsed.map((o: any) => ({ label: o.label || '', content: o.content || '' }))
    } else if (props.questionType === 'fill_blank' && parsed.blank_answers) {
      blankAnswers.value = [...parsed.blank_answers]
    } else if (props.questionType === 'true_false') {
      trueFalseValue.value = !!parsed.is_true
    } else {
      internalValue.value = v
    }
  } catch {
    internalValue.value = v
  }
}, { immediate: true })

watch([options, blankAnswers, trueFalseValue, internalValue], () => {
  let val: any
  if (isChoiceType.value) {
    val = options.value.filter(o => o.label || o.content)
  } else if (props.questionType === 'fill_blank') {
    val = { blank_answers: blankAnswers.value }
  } else if (props.questionType === 'true_false') {
    val = { is_true: trueFalseValue.value }
  } else {
    try { val = JSON.parse(internalValue.value) } catch { val = {} }
  }
  emit('update:modelValue', JSON.stringify(val))
}, { deep: true })

import QuestionJsonEditor from './QuestionJsonEditor.vue'
</script>

<style scoped>
.option-row { display: flex; gap: 8px; align-items: center; margin-bottom: 4px; }
</style>