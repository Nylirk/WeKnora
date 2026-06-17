<template>
  <div class="question-set-detail">
    <div class="detail-header">
      <t-button variant="text" @click="$emit('back')">
        <template #icon><t-icon name="chevron-left" /></template>
        {{ $t('common.back', '返回') }}
      </t-button>
      <h2>{{ setName }}</h2>
      <div class="header-actions">
        <t-button theme="primary" @click="openCreateDialog">
          <template #icon><t-icon name="add" /></template>
          {{ $t('questionBank.addQuestion', '添加题目') }}
        </t-button>
        <t-button @click="importVisible = true">{{ $t('questionBank.import', '导入') }}</t-button>
        <t-button theme="success" @click="exportToEval">{{ $t('questionBank.export', '导出评测集') }}</t-button>
      </div>
    </div>

    <div class="filter-bar">
      <t-select v-model="filter.question_type" :placeholder="$t('questionBank.typeFilter', '题目类型')" clearable style="width: 140px" @change="loadQuestions">
        <t-option v-for="qt in questionTypes" :key="qt" :value="qt" :label="questionTypeLabel(qt)" />
      </t-select>
      <t-select v-model="filter.difficulty" :placeholder="$t('questionBank.difficultyFilter', '难度')" clearable style="width: 100px" @change="loadQuestions">
        <t-option value="easy" :label="$t('questionBank.easy', '简单')" />
        <t-option value="medium" :label="$t('questionBank.medium', '中等')" />
        <t-option value="hard" :label="$t('questionBank.hard', '困难')" />
      </t-select>
      <t-select v-model="filter.status" :placeholder="$t('questionBank.statusFilter', '状态')" clearable style="width: 100px" @change="loadQuestions">
        <t-option value="draft" :label="$t('questionBank.draft', '草稿')" />
        <t-option value="reviewed" :label="$t('questionBank.reviewed', '已审')" />
        <t-option value="rejected" :label="$t('questionBank.rejected', '已拒')" />
      </t-select>
      <t-input v-model="filter.keyword" :placeholder="$t('questionBank.searchPlaceholder', '搜索题干...')" clearable style="width: 200px" @enter="loadQuestions" />
    </div>

    <t-table :data="questions" :loading="loading" row-key="id" hover>
      <t-table-column :title="$t('questionBank.type', '类型')" :width="100">
        <template #default="{ row }">{{ questionTypeLabel(row.question_type) }}</template>
      </t-table-column>
      <t-table-column :title="$t('questionBank.stem', '题干')" prop="stem_text" ellipsis />
      <t-table-column :title="$t('questionBank.difficulty', '难度')" :width="80">
        <template #default="{ row }">{{ difficultyLabel(row.difficulty) }}</template>
      </t-table-column>
      <t-table-column :title="$t('questionBank.status', '状态')" :width="80">
        <template #default="{ row }">
          <t-tag :theme="row.status === 'reviewed' ? 'success' : row.status === 'rejected' ? 'danger' : 'default'" size="small">
            {{ statusLabel(row.status) }}
          </t-tag>
        </template>
      </t-table-column>
      <t-table-column :title="$t('common.action', '操作')" :width="200" fixed="right">
        <template #default="{ row }">
          <t-link theme="primary" @click="openEditDialog(row)">{{ $t('common.edit', '编辑') }}</t-link>
          <t-link v-if="row.status === 'draft'" theme="success" @click="reviewQuestion(row)">{{ $t('questionBank.review', '审核') }}</t-link>
          <t-link theme="danger" @click="removeQuestion(row)">{{ $t('common.delete', '删除') }}</t-link>
        </template>
      </t-table-column>
    </t-table>

    <QuestionEditDialog
      v-model:visible="editVisible"
      :question="editingQuestion"
      :set-id="setId"
      :knowledge-base-id="knowledgeBaseId"
      @saved="loadQuestions"
    />
    <QuestionImportDialog
      v-model:visible="importVisible"
      :set-id="setId"
      :knowledge-base-id="knowledgeBaseId"
      @imported="loadQuestions"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  getQuestionSet, listQuestions, deleteQuestion as apiDeleteQuestion,
  updateQuestionStatus, exportToEvaluationDataset,
  type Question, type QuestionListFilter, type QuestionType,
} from '@/api/question'

const props = defineProps<{ setId: string; knowledgeBaseId: string }>()
defineEmits<{ back: [] }>()

const questionTypes: QuestionType[] = ['single_choice', 'multiple_choice', 'true_false', 'fill_blank', 'short_answer', 'essay', 'composite']
const setName = ref('')
const questions = ref<Question[]>([])
const loading = ref(false)
const filter = ref<QuestionListFilter>({})
const editVisible = ref(false)
const importVisible = ref(false)
const editingQuestion = ref<Question | null>(null)

async function loadQuestions() {
  loading.value = true
  try {
    const res = await listQuestions(props.knowledgeBaseId, props.setId, filter.value, 1, 200)
    questions.value = res.data || []
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载题目失败')
  } finally {
    loading.value = false
  }
}

function openCreateDialog() {
  editingQuestion.value = null
  editVisible.value = true
}

function openEditDialog(q: Question) {
  editingQuestion.value = q
  editVisible.value = true
}

async function reviewQuestion(q: Question) {
  try {
    await updateQuestionStatus(props.knowledgeBaseId, props.setId, q.id, { status: 'reviewed' })
    MessagePlugin.success('审核通过')
    await loadQuestions()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '审核失败')
  }
}

async function removeQuestion(q: Question) {
  try {
    await apiDeleteQuestion(props.knowledgeBaseId, props.setId, q.id)
    MessagePlugin.success('删除成功')
    await loadQuestions()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

async function exportToEval() {
  const name = prompt('请输入评测集名称', setName.value)
  if (!name) return
  try {
    await exportToEvaluationDataset(props.knowledgeBaseId, props.setId, { name })
    MessagePlugin.success('导出成功')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '导出失败')
  }
}

function questionTypeLabel(t: QuestionType) {
  const map: Record<QuestionType, string> = {
    single_choice: '单选', multiple_choice: '多选', true_false: '判断',
    fill_blank: '填空', short_answer: '简答', essay: '论述', composite: '复合',
  }
  return map[t] || t
}
function difficultyLabel(d: string) {
  const map: Record<string, string> = { easy: '简单', medium: '中等', hard: '困难' }
  return map[d] || d
}
function statusLabel(s: string) {
  const map: Record<string, string> = { draft: '草稿', reviewed: '已审', rejected: '已拒' }
  return map[s] || s
}

onMounted(async () => {
  try {
    const set = await getQuestionSet(props.knowledgeBaseId, props.setId)
    setName.value = set.name
  } catch { /* ignore */ }
  await loadQuestions()
})

import QuestionEditDialog from './QuestionEditDialog.vue'
import QuestionImportDialog from './QuestionImportDialog.vue'
</script>

<style scoped>
.question-set-detail { padding: 16px; }
.detail-header { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }
.detail-header h2 { flex: 1; margin: 0; }
.header-actions { display: flex; gap: 8px; }
.filter-bar { display: flex; gap: 8px; margin-bottom: 16px; flex-wrap: wrap; }
</style>