<template>
  <t-dialog v-model:visible="dialogVisible" :header="isEdit ? $t('questionBank.editQuestion', '编辑题目') : $t('questionBank.addQuestion', '添加题目')" :width="700" @confirm="save" @close="dialogVisible = false">
    <t-form label-align="top">
      <t-form-item :label="$t('questionBank.type', '题目类型')">
        <t-select v-model="form.question_type" :disabled="isEdit">
          <t-option v-for="qt in questionTypes" :key="qt" :value="qt" :label="questionTypeLabel(qt)" />
        </t-select>
      </t-form-item>
      <t-form-item :label="$t('questionBank.stem', '题干')" :required="true">
        <t-textarea v-model="form.stem_text" :placeholder="$t('questionBank.stemPlaceholder', '请输入题干')" :autosize="{ minRows: 2, maxRows: 6 }" />
      </t-form-item>
      <t-form-item :label="$t('questionBank.difficulty', '难度')">
        <t-select v-model="form.difficulty">
          <t-option value="easy" :label="$t('questionBank.easy', '简单')" />
          <t-option value="medium" :label="$t('questionBank.medium', '中等')" />
          <t-option value="hard" :label="$t('questionBank.hard', '困难')" />
        </t-select>
      </t-form-item>
      <t-form-item :label="$t('questionBank.answerText', '答案文本')">
        <t-textarea v-model="form.answer_text" :placeholder="$t('questionBank.answerPlaceholder', '请输入答案')" :autosize="{ minRows: 2, maxRows: 6 }" />
      </t-form-item>
      <t-form-item :label="$t('questionBank.analysis', '解析')">
        <t-textarea v-model="form.analysis_text" :placeholder="$t('questionBank.analysisPlaceholder', '可选解析')" :autosize="{ minRows: 2 }" />
      </t-form-item>

      <t-form-item :label="$t('questionBank.questionBody', '题目结构 (JSON)')">
        <QuestionTypeForm :question-type="form.question_type" v-model="form.question_body" />
      </t-form-item>
      <t-form-item :label="$t('questionBank.answerBody', '答案结构 (JSON)')">
        <QuestionJsonEditor v-model="form.answer_body" />
      </t-form-item>
    </t-form>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  createQuestion, updateQuestion, type Question, type QuestionType,
} from '@/api/question'

const props = defineProps<{ visible: boolean; question: Question | null; setId: string; knowledgeBaseId: string }>()
const emit = defineEmits<{ 'update:visible': [v: boolean]; saved: [] }>()

const dialogVisible = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
})

const isEdit = computed(() => !!props.question)
const questionTypes: QuestionType[] = ['single_choice', 'multiple_choice', 'true_false', 'fill_blank', 'short_answer', 'essay', 'composite']

const defaultForm = () => ({
  question_type: 'single_choice' as QuestionType,
  stem_text: '',
  answer_text: '',
  analysis_text: '',
  difficulty: 'medium',
  question_body: '{}',
  answer_body: '{}',
  grading_rubric: '{}',
  knowledge_points: '[]',
  tags: '[]',
  source_knowledge_id: '',
  evidence_chunk_ids: '[]',
  sort_order: 0,
})

const form = ref(defaultForm())

watch(() => props.visible, (v) => {
  if (v && props.question) {
    const q = props.question
    form.value = {
      question_type: q.question_type,
      stem_text: q.stem_text || '',
      answer_text: q.answer_text || '',
      analysis_text: q.analysis_text || '',
      difficulty: q.difficulty || 'medium',
      question_body: typeof q.question_body === 'string' ? q.question_body : JSON.stringify(q.question_body || {}),
      answer_body: typeof q.answer_body === 'string' ? q.answer_body : JSON.stringify(q.answer_body || {}),
      grading_rubric: typeof q.grading_rubric === 'string' ? q.grading_rubric : JSON.stringify(q.grading_rubric || {}),
      knowledge_points: typeof q.knowledge_points === 'string' ? q.knowledge_points : JSON.stringify(q.knowledge_points || []),
      tags: typeof q.tags === 'string' ? q.tags : JSON.stringify(q.tags || []),
      source_knowledge_id: q.source_knowledge_id || '',
      evidence_chunk_ids: typeof q.evidence_chunk_ids === 'string' ? q.evidence_chunk_ids : JSON.stringify(q.evidence_chunk_ids || []),
      sort_order: q.sort_order || 0,
    }
  } else if (v) {
    form.value = defaultForm()
  }
})

async function save() {
  if (!form.value.stem_text.trim()) {
    MessagePlugin.warning('题干不能为空')
    return
  }
  try {
    const payload: Record<string, unknown> = {
      question_type: form.value.question_type,
      stem_text: form.value.stem_text,
      answer_text: form.value.answer_text,
      analysis_text: form.value.analysis_text,
      difficulty: form.value.difficulty,
      question_body: safeParse(form.value.question_body),
      answer_body: safeParse(form.value.answer_body),
      grading_rubric: safeParse(form.value.grading_rubric),
      knowledge_points: safeParse(form.value.knowledge_points),
      tags: safeParse(form.value.tags),
      source_knowledge_id: form.value.source_knowledge_id,
      evidence_chunk_ids: safeParse(form.value.evidence_chunk_ids),
      sort_order: form.value.sort_order,
    }
    if (isEdit.value && props.question) {
      await updateQuestion(props.knowledgeBaseId, props.setId, props.question.id, payload)
      MessagePlugin.success('更新成功')
    } else {
      await createQuestion(props.knowledgeBaseId, props.setId, payload)
      MessagePlugin.success('添加成功')
    }
    dialogVisible.value = false
    emit('saved')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  }
}

function safeParse(s: string) {
  try { return JSON.parse(s) } catch { return {} }
}

function questionTypeLabel(t: QuestionType) {
  const map: Record<QuestionType, string> = {
    single_choice: '单选', multiple_choice: '多选', true_false: '判断',
    fill_blank: '填空', short_answer: '简答', essay: '论述', composite: '复合',
  }
  return map[t] || t
}

import QuestionTypeForm from './QuestionTypeForm.vue'
import QuestionJsonEditor from './QuestionJsonEditor.vue'
</script>