<template>
  <div class="question-set-detail">
    <div class="detail-header">
      <h2>{{ displaySetName }}</h2>
      <div class="header-actions">
        <t-button theme="primary" @click="openCreateDialog">
          <template #icon><t-icon name="add" /></template>
          {{ $t('questionBank.addQuestion', '新增题目') }}
        </t-button>
        <t-popup
          v-model:visible="headerImportMenuVisible"
          trigger="click"
          placement="bottom-right"
          overlay-class-name="question-import-type-popup"
        >
          <t-button>{{ $t('questionBank.import') }}</t-button>
          <template #content>
            <div class="import-type-menu">
              <button type="button" class="import-type-item" @click="openSingleImport">
                <span class="import-type-title">单个导入</span>
                <span class="import-type-description">导入一个文件并进入题目审核工作台</span>
              </button>
              <button type="button" class="import-type-item" disabled>
                <span class="import-type-title">批量导入</span>
                <span class="import-type-description">即将支持</span>
              </button>
            </div>
          </template>
        </t-popup>
        <t-button @click="generateVisible = true">{{ $t('questionBank.generateTitle', '生成题目') }}</t-button>
        <t-button theme="success" @click="openExportDialog">{{ $t('questionBank.export', '导出评测集') }}</t-button>
      </div>
    </div>

    <div class="filter-bar">
      <t-select v-model="filter.question_type" :placeholder="$t('questionBank.typeFilter', '题型')" clearable style="width: 120px" @change="reloadFromFirstPage">
        <t-option v-for="qt in questionTypes" :key="qt" :value="qt" :label="questionTypeLabel(qt)" />
      </t-select>
      <t-select v-model="filter.status" :placeholder="$t('questionBank.statusFilter', '状态')" clearable style="width: 100px" @change="reloadFromFirstPage">
        <t-option value="draft" :label="$t('questionBank.draft', '草稿')" />
        <t-option value="reviewed" :label="$t('questionBank.reviewed', '已审')" />
        <t-option value="rejected" :label="$t('questionBank.rejected', '已拒')" />
      </t-select>
      <t-select v-model="filter.difficulty" :placeholder="$t('questionBank.difficultyFilter', '难度')" clearable style="width: 100px" @change="reloadFromFirstPage">
        <t-option value="easy" :label="$t('questionBank.easy', '简单')" />
        <t-option value="medium" :label="$t('questionBank.medium', '中等')" />
        <t-option value="hard" :label="$t('questionBank.hard', '困难')" />
      </t-select>
      <t-input v-model="filter.knowledge_point" placeholder="知识点" clearable style="width: 140px" @clear="reloadFromFirstPage" @enter="reloadFromFirstPage" />
      <t-input v-model="filter.tag" placeholder="标签" clearable style="width: 120px" @clear="reloadFromFirstPage" @enter="reloadFromFirstPage" />
      <t-input v-model="filter.keyword" :placeholder="$t('questionBank.searchPlaceholder', '搜索题干...')" clearable style="width: 180px" @clear="reloadFromFirstPage" @enter="reloadFromFirstPage" />
    </div>

    <!-- Batch action bar -->
    <div v-if="selectedRowKeys.length" class="batch-actions">
      <span class="batch-label">已选择 {{ selectedRowKeys.length }} 题</span>
      <t-button size="small" variant="outline" @click="batchReview">批量审核</t-button>
      <t-popconfirm content="确定要删除选中题目？此操作不可撤销。" @confirm="batchDelete">
        <t-button size="small" variant="outline" theme="danger">批量删除</t-button>
      </t-popconfirm>
      <t-button size="small" variant="text" @click="selectedRowKeys = []">清空选择</t-button>
    </div>

    <t-table
      v-if="loading || questions.length > 0"
      :data="questions"
      :columns="questionColumns"
      :loading="loading"
      :selected-row-keys="selectedRowKeys"
      :pagination="{ current: currentPage, pageSize, total: questionTotal, showJumper: true, showPageSize: true, pageSizeOptions: [20, 50, 100, 200] }"
      row-key="id"
      hover
      @select-change="onSelectChange"
      @page-change="onPageChange"
    >
      <template #question_type="{ row }">
        {{ questionTypeLabel(row.question_type) }}
      </template>
      <template #difficulty="{ row }">
        {{ difficultyLabel(row.difficulty) }}
      </template>
      <template #status="{ row }">
        <t-tooltip v-if="row.status === 'reviewed' && row.reviewed_at" :content="`审核人：${row.reviewed_by || '未知'}\n审核时间：${row.reviewed_at}`">
          <t-tag theme="success" size="small">{{ statusLabel(row.status) }}</t-tag>
        </t-tooltip>
        <t-link v-else-if="row.status === 'draft'" theme="primary" hover="color" @click="reviewSingleQuestion(row)">
          <t-tag theme="default" size="small" class="draft-review-tag">{{ statusLabel(row.status) }}</t-tag>
        </t-link>
        <t-tag v-else :theme="row.status === 'rejected' ? 'danger' : 'default'" size="small">
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
    <t-empty v-else description="当前题集暂无题目" class="question-empty" />

    <QuestionEditDialog
      v-model:visible="editVisible"
      :question="editingQuestion"
      :set-id="setId"
      :knowledge-base-id="knowledgeBaseId"
      @saved="refreshAfterMutation"
    />
    <QuestionFileImportDialog
      :key="fileImportSession"
      v-model:visible="fileImportVisible"
      :set-id="setId"
      :knowledge-base-id="knowledgeBaseId"
      import-mode="single"
      @parsed="handleFileParsed"
    />
    <QuestionImportWorkbench
      v-model:visible="workbenchVisible"
      :kb-id="knowledgeBaseId"
      :set-id="setId"
      @imported="handleWorkbenchImported"
      @abandoned="handleWorkbenchAbandoned"
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
    <t-dialog
      v-model:visible="restoreDraftVisible"
      header="发现未完成的导入草稿"
      attach="body"
      :z-index="3200"
      :close-btn="false"
      :close-on-overlay-click="false"
      :close-on-esc-keydown="false"
      :confirm-btn="{ content: '恢复草稿', theme: 'primary', loading: restoringDraft }"
      :cancel-btn="{ content: '重新导入', loading: startingFreshImport }"
      @confirm="restoreImportDraft"
      @cancel="startFreshImport"
    >
      <p class="restore-draft-copy">
        该题集存在 7 天内保存的导入草稿（{{ pendingDraftTime }}），是否继续处理？
      </p>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, nextTick, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  getQuestionSet, listQuestions, deleteQuestion as apiDeleteQuestion,
  exportToEvaluationDataset, updateQuestionStatus,
  type Question, type QuestionListFilter, type QuestionType,
} from '@/api/question'
import type { BlockPreviewSummary, ImportBlock } from '@/api/question_block'
import { useImportWorkbenchStore } from '@/stores/importWorkbench'
import {
  cleanExpiredDrafts, deleteDraft, loadDraft, saveDraft, type ImportDraft,
} from '@/utils/importDraftDB'
import { resolveQuestionRows, resolveQuestionTotal } from '../questionData'

const props = defineProps<{ setId: string; knowledgeBaseId: string; setName?: string }>()
const emit = defineEmits<{ generated: []; changed: [total: number] }>()
const workbenchStore = useImportWorkbenchStore()

const questionTypes: QuestionType[] = ['single_choice', 'multiple_choice', 'true_false', 'fill_blank', 'short_answer', 'essay', 'composite']
const questionColumns = computed(() => [
  { colKey: 'row-select', type: 'multiple' as const, width: 50 },
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
const fileImportVisible = ref(false)
const fileImportSession = ref(0)
const workbenchVisible = ref(false)
const restoreDraftVisible = ref(false)
const pendingDraft = ref<ImportDraft | null>(null)
const restoringDraft = ref(false)
const startingFreshImport = ref(false)
const pendingDraftTime = computed(() => pendingDraft.value
  ? new Date(pendingDraft.value.timestamp).toLocaleString()
  : '')
const headerImportMenuVisible = ref(false)
const generateVisible = ref(false)
const exportVisible = ref(false)
const exportName = ref('')
const exportDescription = ref('')
const exporting = ref(false)
const editingQuestion = ref<Question | null>(null)
const selectedRowKeys = ref<string[]>([])
const currentPage = ref(1)
const pageSize = ref(50)
const questionTotal = ref(0)

function onSelectChange(value: string[]) {
  selectedRowKeys.value = value
}

function onPageChange(pageInfo: { current: number; pageSize: number }) {
  currentPage.value = pageInfo.current
  pageSize.value = pageInfo.pageSize
  selectedRowKeys.value = []
  loadQuestions()
}

function reloadFromFirstPage() {
  currentPage.value = 1
  selectedRowKeys.value = []
  loadQuestions()
}

async function reviewSingleQuestion(row: Question) {
  if (row.status !== 'draft') return
  try {
    await updateQuestionStatus(props.knowledgeBaseId, props.setId, row.id, { status: 'reviewed' })
    MessagePlugin.success('审核成功')
    await refreshAfterMutation()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '审核失败')
  }
}

async function batchReview() {
  const draftIds = selectedRowKeys.value.filter(id => {
    const q = questions.value.find(q => q.id === id)
    return q?.status === 'draft'
  })
  if (!draftIds.length) {
    MessagePlugin.warning('没有可审核的草稿题目')
    return
  }
  let done = 0; let failed = 0
  for (const id of draftIds) {
    try {
      await updateQuestionStatus(props.knowledgeBaseId, props.setId, id, { status: 'reviewed' })
      done++
    } catch { failed++ }
  }
  MessagePlugin.success(`审核完成：成功 ${done} 题` + (failed ? `，失败 ${failed} 题` : ''))
  selectedRowKeys.value = []
  await refreshAfterMutation()
}

async function batchDelete() {
  if (!selectedRowKeys.value.length) return
  let done = 0; let failed = 0
  for (const id of selectedRowKeys.value) {
    try {
      await apiDeleteQuestion(props.knowledgeBaseId, props.setId, id)
      done++
    } catch { failed++ }
  }
  MessagePlugin.success(`删除完成：成功 ${done} 题` + (failed ? `，失败 ${failed} 题` : ''))
  selectedRowKeys.value = []
  await refreshAfterMutation()
}

async function loadQuestions(): Promise<number | null> {
  loading.value = true
  try {
    const res = await listQuestions(props.knowledgeBaseId, props.setId, filter.value, currentPage.value, pageSize.value)
    const rows = resolveQuestionRows<Question>(res)
    const total = resolveQuestionTotal(res, rows)
    questions.value = rows
    questionTotal.value = total
    return total
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

async function closeAllImportMenus() {
  headerImportMenuVisible.value = false
  await nextTick()
}

function closeImportModals() {
  fileImportVisible.value = false
  restoreDraftVisible.value = false
}

async function openSingleImport() {
  await closeAllImportMenus()
  closeImportModals()
  await nextTick()

  try {
    await cleanExpiredDrafts()
    const draft = await loadDraft(props.knowledgeBaseId, props.setId)
    if (draft) {
      pendingDraft.value = draft
      restoreDraftVisible.value = true
      return
    }
  } catch (error: any) {
    MessagePlugin.warning(error?.message || '读取导入草稿失败，将开始新的导入。')
  }

  await openFileImportDialog()
}

async function openFileImportDialog() {
  closeImportModals()
  await nextTick()

  pendingDraft.value = null
  fileImportSession.value += 1
  fileImportVisible.value = true
}

function applyDraftToWorkbench(draft: ImportDraft) {
  workbenchStore.reset()
  workbenchStore.kbId = props.knowledgeBaseId
  workbenchStore.setId = props.setId
  workbenchStore.strategyPreset = draft.strategyPreset
  workbenchStore.defaultDifficulty = draft.defaultDifficulty
  workbenchStore.importMode = draft.importMode as 'single' | 'batch'
  workbenchStore.importFormat = (draft.importFormat as 'json' | 'word' | 'pdf') || 'word'
  workbenchStore.setBlocksFromResponse(draft.blocks)
  workbenchStore.questions = draft.questions ?? []
  workbenchStore.currentStep = draft.currentStep || 'block-review'
  workbenchStore.draftExists = true
}

async function restoreImportDraft() {
  if (restoringDraft.value) return
  restoringDraft.value = true
  try {
    const draft = pendingDraft.value
    if (!draft || !Array.isArray(draft.blocks) || draft.blocks.length === 0) {
      MessagePlugin.warning('草稿中没有可恢复的 blocks，请重新导入。')
      await startFreshImport()
      return
    }
    fileImportVisible.value = false
    restoreDraftVisible.value = false
    headerImportMenuVisible.value = false
    applyDraftToWorkbench(draft)
    pendingDraft.value = null
    await nextTick()
    workbenchVisible.value = true
  } finally {
    restoringDraft.value = false
  }
}

async function startFreshImport() {
  if (startingFreshImport.value) return
  startingFreshImport.value = true
  try {
    closeImportModals()
    pendingDraft.value = null
    await deleteDraft(props.knowledgeBaseId, props.setId)
    restoreDraftVisible.value = false
    await nextTick()
    await openFileImportDialog()
  } finally {
    startingFreshImport.value = false
  }
}

async function handleFileParsed(payload: {
  blocks: ImportBlock[]
  summary: BlockPreviewSummary
  strategyPreset: string
  importFormat: 'json' | 'word' | 'pdf'
  importMode: 'single' | 'batch'
}) {
  fileImportVisible.value = false
  restoreDraftVisible.value = false
  headerImportMenuVisible.value = false
  pendingDraft.value = null
  workbenchStore.reset()
  workbenchStore.kbId = props.knowledgeBaseId
  workbenchStore.setId = props.setId
  workbenchStore.strategyPreset = payload.strategyPreset
  workbenchStore.defaultDifficulty = 'medium'
  workbenchStore.importMode = payload.importMode
  workbenchStore.importFormat = payload.importFormat
  workbenchStore.setBlocksFromResponse(payload.blocks)

  try {
    await saveDraft({
      kbId: props.knowledgeBaseId,
      setId: props.setId,
      blocks: payload.blocks,
      strategyPreset: payload.strategyPreset,
      defaultDifficulty: workbenchStore.defaultDifficulty,
      importMode: payload.importMode,
      importFormat: payload.importFormat,
      currentStep: 'block-review',
      questions: [],
      timestamp: Date.now(),
    })
  } catch (error: any) {
    MessagePlugin.warning(error?.message || '草稿保存失败，本次仍可继续处理。')
  }

  await nextTick()
  workbenchVisible.value = true
}

async function handleWorkbenchImported() {
  workbenchVisible.value = false
  await refreshAfterMutation()
}

function handleWorkbenchAbandoned() {
  workbenchVisible.value = false
}

// Guard: if any import dialog opens, close the popup menu
watch(fileImportVisible, (fileVisible) => {
  if (fileVisible) {
    headerImportMenuVisible.value = false
  }
})

function handleGenerated() {
  emit('generated')
}

async function refreshAfterMutation() {
  selectedRowKeys.value = []
  const total = await loadQuestions()
  // If current page is empty and past page 1, go back one page
  if (total !== null && questions.value.length === 0 && currentPage.value > 1) {
    currentPage.value -= 1
    await loadQuestions()
  }
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
import QuestionFileImportDialog from './QuestionFileImportDialog.vue'
import QuestionGenerateDialog from './QuestionGenerateDialog.vue'
import QuestionImportWorkbench from '../QuestionImportWorkbench.vue'
</script>

<style scoped>
.question-set-detail { min-width: 0; }
.detail-header { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }
.detail-header h2 { flex: 1; margin: 0; }
.header-actions { display: flex; gap: 8px; flex-wrap: wrap; justify-content: flex-end; }
.filter-bar { display: flex; gap: 8px; margin-bottom: 16px; flex-wrap: wrap; }
.batch-actions { display: flex; align-items: center; gap: 8px; padding: 6px 12px; margin-bottom: 8px; background: var(--td-bg-color-secondarycontainer); border-radius: 6px; }
.batch-label { font-size: 13px; color: var(--td-text-color-secondary); margin-right: 8px; }
.draft-review-tag { cursor: pointer; }
.draft-review-tag:hover { color: var(--td-brand-color); }
.question-empty { padding: 48px 16px; }
.restore-draft-copy { margin: 0; color: var(--td-text-color-secondary); line-height: 1.7; }
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
