<template>
  <div class="question-set-detail">
    <div class="detail-header">
      <h2>{{ displaySetName }}</h2>
      <div class="header-actions">
        <t-button theme="primary" @click="openCreateDialog">
          <template #icon><t-icon name="add" /></template>
          {{ $t('questionBank.addQuestion', '新增题目') }}
        </t-button>
        <t-popup trigger="click" placement="bottom-right" overlay-class-name="question-import-type-popup">
          <t-button>{{ $t('questionBank.import') }}</t-button>
          <template #content>
            <div class="import-type-menu">
              <button type="button" class="import-type-item" @click="openJsonImport">
                <span class="import-type-title">{{ $t('questionBank.jsonImport') }}</span>
                <span class="import-type-description">{{ $t('questionBank.jsonImportDescription') }}</span>
              </button>
              <button type="button" class="import-type-item" @click="openFileImport('word')">
                <span class="import-type-title">{{ $t('questionBank.wordImport') }}</span>
                <span class="import-type-description">{{ $t('questionBank.wordImportDescription') }}</span>
              </button>
              <button type="button" class="import-type-item" @click="openFileImport('pdf')">
                <span class="import-type-title">{{ $t('questionBank.pdfImport') }}</span>
                <span class="import-type-description">{{ $t('questionBank.pdfImportDescription') }}</span>
              </button>
            </div>
          </template>
        </t-popup>
        <t-button @click="generateVisible = true">{{ $t('questionBank.generateTitle', '生成题目') }}</t-button>
        <t-button theme="success" @click="openExportDialog">{{ $t('questionBank.export', '导出评测集') }}</t-button>
      </div>
    </div>

    <div class="filter-bar">
      <t-select v-model="filter.question_type" :placeholder="$t('questionBank.typeFilter', '题型')" clearable style="width: 120px" @change="loadQuestions">
        <t-option v-for="qt in questionTypes" :key="qt" :value="qt" :label="questionTypeLabel(qt)" />
      </t-select>
      <t-select v-model="filter.status" :placeholder="$t('questionBank.statusFilter', '状态')" clearable style="width: 100px" @change="loadQuestions">
        <t-option value="draft" :label="$t('questionBank.draft', '草稿')" />
        <t-option value="reviewed" :label="$t('questionBank.reviewed', '已审')" />
        <t-option value="rejected" :label="$t('questionBank.rejected', '已拒')" />
      </t-select>
      <t-select v-model="filter.difficulty" :placeholder="$t('questionBank.difficultyFilter', '难度')" clearable style="width: 100px" @change="loadQuestions">
        <t-option value="easy" :label="$t('questionBank.easy', '简单')" />
        <t-option value="medium" :label="$t('questionBank.medium', '中等')" />
        <t-option value="hard" :label="$t('questionBank.hard', '困难')" />
      </t-select>
      <t-input v-model="filter.knowledge_point" placeholder="知识点" clearable style="width: 140px" @clear="loadQuestions" @enter="loadQuestions" />
      <t-input v-model="filter.tag" placeholder="标签" clearable style="width: 120px" @clear="loadQuestions" @enter="loadQuestions" />
      <t-input v-model="filter.keyword" :placeholder="$t('questionBank.searchPlaceholder', '搜索题干...')" clearable style="width: 180px" @clear="loadQuestions" @enter="loadQuestions" />
    </div>

    <t-table
      v-if="loading || questions.length > 0"
      :data="questions"
      :columns="questionColumns"
      :loading="loading"
      row-key="id"
      hover
    >
      <template #question_type="{ row }">
        {{ questionTypeLabel(row.question_type) }}
      </template>
      <template #difficulty="{ row }">
        {{ difficultyLabel(row.difficulty) }}
      </template>
      <template #status="{ row }">
          <t-tag :theme="row.status === 'reviewed' ? 'success' : row.status === 'rejected' ? 'danger' : 'default'" size="small">
            {{ statusLabel(row.status) }}
          </t-tag>
      </template>
      <template #operation="{ row }">
        <t-space size="small">
          <t-link theme="primary" @click="openEditDialog(row)">{{ $t('common.edit', '编辑') }}</t-link>
          <t-link theme="danger" @click="removeQuestion(row)">{{ $t('common.delete', '删除') }}</t-link>
        </t-space>
      </template>
    </t-table>
    <t-empty v-else description="当前题集暂无题目" class="question-empty">
      <template #action>
        <t-space>
          <t-button theme="primary" @click="openCreateDialog">新增题目</t-button>
          <t-popup trigger="click" placement="bottom" overlay-class-name="question-import-type-popup">
            <t-button>{{ $t('questionBank.import') }}</t-button>
            <template #content>
              <div class="import-type-menu">
                <button type="button" class="import-type-item" @click="openJsonImport">
                  <span class="import-type-title">{{ $t('questionBank.jsonImport') }}</span>
                  <span class="import-type-description">{{ $t('questionBank.jsonImportDescription') }}</span>
                </button>
                <button type="button" class="import-type-item" @click="openFileImport('word')">
                  <span class="import-type-title">{{ $t('questionBank.wordImport') }}</span>
                  <span class="import-type-description">{{ $t('questionBank.wordImportDescription') }}</span>
                </button>
                <button type="button" class="import-type-item" @click="openFileImport('pdf')">
                  <span class="import-type-title">{{ $t('questionBank.pdfImport') }}</span>
                  <span class="import-type-description">{{ $t('questionBank.pdfImportDescription') }}</span>
                </button>
              </div>
            </template>
          </t-popup>
        </t-space>
      </template>
    </t-empty>

    <QuestionEditDialog
      v-model:visible="editVisible"
      :question="editingQuestion"
      :set-id="setId"
      :knowledge-base-id="knowledgeBaseId"
      @saved="refreshAfterMutation"
    />
    <QuestionImportDialog
      v-model:visible="importVisible"
      :set-id="setId"
      :knowledge-base-id="knowledgeBaseId"
      :current-questions="questions"
      @imported="refreshAfterMutation"
    />
    <QuestionFileImportDialog
      :key="`${fileImportType}-${fileImportSession}`"
      v-model:visible="fileImportVisible"
      :set-id="setId"
      :knowledge-base-id="knowledgeBaseId"
      :import-type="fileImportType"
      :current-questions="questions"
      @imported="refreshAfterMutation"
    />
    <QuestionGenerateDialog
      v-model:visible="generateVisible"
      :knowledge-base-id="knowledgeBaseId"
      @generated="handleGenerated"
    />
    <t-dialog
      v-model:visible="exportVisible"
      header="导出评测集"
      :confirm-btn="{ content: '确认导出', loading: exporting }"
      :cancel-btn="{ content: '取消', disabled: exporting }"
      @confirm="confirmExport"
    >
      <t-form label-align="top">
        <t-form-item label="评测集名称" :required="true">
          <t-input v-model="exportName" placeholder="请输入评测集名称" />
        </t-form-item>
        <t-form-item label="描述">
          <t-textarea v-model="exportDescription" placeholder="可选描述" :autosize="{ minRows: 3, maxRows: 6 }" />
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  getQuestionSet, listQuestions, deleteQuestion as apiDeleteQuestion,
  exportToEvaluationDataset,
  type Question, type QuestionListFilter, type QuestionType,
} from '@/api/question'
import { resolveQuestionRows, resolveQuestionTotal } from '../questionData'

const props = defineProps<{ setId: string; knowledgeBaseId: string; setName?: string }>()
const emit = defineEmits<{ generated: []; changed: [total: number] }>()

const questionTypes: QuestionType[] = ['single_choice', 'multiple_choice', 'true_false', 'fill_blank', 'short_answer', 'essay', 'composite']
const questionColumns = computed(() => [
  { colKey: 'question_type', title: '类型', width: 100, cell: 'question_type' },
  { colKey: 'stem_text', title: '题干', ellipsis: true },
  { colKey: 'difficulty', title: '难度', width: 80, cell: 'difficulty' },
  { colKey: 'status', title: '状态', width: 90, cell: 'status' },
  { colKey: 'operation', title: '操作', width: 120, fixed: 'right', cell: 'operation' },
])
const fetchedSetName = ref('')
const displaySetName = computed(() => props.setName?.trim() || fetchedSetName.value)
const questions = ref<Question[]>([])
const loading = ref(false)
const filter = ref<QuestionListFilter>({})
const editVisible = ref(false)
const importVisible = ref(false)
const fileImportVisible = ref(false)
const fileImportType = ref<'word' | 'pdf'>('word')
const fileImportSession = ref(0)
const generateVisible = ref(false)
const exportVisible = ref(false)
const exportName = ref('')
const exportDescription = ref('')
const exporting = ref(false)
const editingQuestion = ref<Question | null>(null)

async function loadQuestions(): Promise<number | null> {
  loading.value = true
  try {
    const res = await listQuestions(props.knowledgeBaseId, props.setId, filter.value, 1, 200)
    const rows = resolveQuestionRows<Question>(res)
    questions.value = rows
    return resolveQuestionTotal(res, rows)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载题目失败')
    questions.value = []
    return null
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

function openJsonImport() {
  importVisible.value = true
}

function openFileImport(type: 'word' | 'pdf') {
  fileImportType.value = type
  fileImportSession.value += 1
  fileImportVisible.value = true
}

function handleGenerated() {
  emit('generated')
}

async function refreshAfterMutation() {
  filter.value = {}
  const total = await loadQuestions()
  if (total !== null) emit('changed', total)
}

async function removeQuestion(q: Question) {
  try {
    await apiDeleteQuestion(props.knowledgeBaseId, props.setId, q.id)
    MessagePlugin.success('删除成功')
    await refreshAfterMutation()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

function openExportDialog() {
  exportName.value = displaySetName.value
  exportDescription.value = ''
  exportVisible.value = true
}

async function confirmExport() {
  const name = exportName.value.trim()
  if (!name) {
    MessagePlugin.warning('请输入评测集名称')
    return
  }
  exporting.value = true
  try {
    await exportToEvaluationDataset(props.knowledgeBaseId, props.setId, {
      name,
      description: exportDescription.value.trim(),
    })
    MessagePlugin.success('导出成功')
    exportVisible.value = false
  } catch (e: any) {
    MessagePlugin.error(e?.message || '导出失败')
  } finally {
    exporting.value = false
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
  if (!props.setName) {
    try {
      const set = await getQuestionSet(props.knowledgeBaseId, props.setId)
      fetchedSetName.value = set.name
    } catch { /* ignore */ }
  }
  await loadQuestions()
})

import QuestionEditDialog from './QuestionEditDialog.vue'
import QuestionImportDialog from './QuestionImportDialog.vue'
import QuestionFileImportDialog from './QuestionFileImportDialog.vue'
import QuestionGenerateDialog from './QuestionGenerateDialog.vue'
</script>

<style scoped>
.question-set-detail { min-width: 0; }
.detail-header { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }
.detail-header h2 { flex: 1; margin: 0; }
.header-actions { display: flex; gap: 8px; flex-wrap: wrap; justify-content: flex-end; }
.filter-bar { display: flex; gap: 8px; margin-bottom: 16px; flex-wrap: wrap; }
.question-empty { padding: 48px 16px; }
.import-type-menu { width: 320px; padding: 6px; }
.import-type-item { width: 100%; display: flex; flex-direction: column; align-items: flex-start; gap: 3px; padding: 10px 12px; border: 0; border-radius: 6px; color: var(--td-text-color-primary); background: transparent; text-align: left; cursor: pointer; }
.import-type-item:not(:disabled):hover { background: var(--td-bg-color-container-hover); }
.import-type-item:disabled { color: var(--td-text-color-disabled); cursor: not-allowed; }
.import-type-title { display: flex; align-items: center; gap: 8px; font-weight: 500; }
.import-type-description,
.import-type-help { color: var(--td-text-color-secondary); font-size: 12px; line-height: 1.5; }
.import-type-item:disabled .import-type-description,
.import-type-item:disabled .import-type-help { color: var(--td-text-color-disabled); }
</style>
