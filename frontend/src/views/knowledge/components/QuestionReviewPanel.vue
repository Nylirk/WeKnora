<template>
  <div class="question-review-panel">
    <div class="stats-bar" v-if="store.questionStats.detected_questions > 0">
      <t-space size="small">
        <t-tag variant="light">识别 {{ store.questionStats.detected_questions }} 题</t-tag>
        <t-tag theme="success" variant="light">有答案 {{ store.questionStats.with_answer }}</t-tag>
        <t-tag theme="warning" variant="light">缺答案 {{ store.questionStats.without_answer }}</t-tag>
      </t-space>
    </div>

    <div v-if="store.questionWarnings.length" class="warnings-box">
      <t-alert theme="warning" :close-btn="false">
        <div v-for="(w, i) in store.questionWarnings" :key="i">{{ w }}</div>
      </t-alert>
    </div>
    <div v-if="store.questionErrors.length" class="errors-box">
      <t-alert theme="error" :close-btn="false">
        <div v-for="(e, i) in store.questionErrors" :key="i">#{{ e.line_number }}: {{ e.message }}</div>
      </t-alert>
    </div>

    <div v-if="store.isParsing" class="parsing-state">
      <t-loading text="解析中…" />
    </div>

    <div v-else-if="store.questions.length === 0 && !store.isParsing" class="empty-state">
      <t-empty description="点击上方「下一步：题目解析」生成题目预览" />
    </div>

    <div v-else class="question-list">
      <div v-for="(item, index) in store.questions" :key="index" class="question-item">
        <div class="question-item-header">
          <t-tag size="small">{{ questionTypeLabel(item.question_type) }}</t-tag>
          <t-tag size="small" variant="light">{{ difficultyLabel(item.difficulty) }}</t-tag>
          <span v-if="item.tags && item.tags.length" class="question-tags">
            <t-tag v-for="(t, ti) in item.tags" :key="ti" size="small" variant="outline">{{ typeof t === 'string' ? t : '' }}</t-tag>
          </span>
          <t-space size="small" style="margin-left: auto">
            <t-button size="small" variant="text" @click="editItem(index)">编辑</t-button>
            <t-button size="small" variant="text" theme="danger" @click="removeItem(index)">移除</t-button>
          </t-space>
        </div>
        <div class="question-stem">{{ item.stem_text }}</div>
        <div v-if="item.answer_text" class="question-answer"><span class="answer-label">答案：</span>{{ item.answer_text }}</div>
        <div v-if="item.analysis_text" class="question-analysis"><span class="analysis-label">解析：</span>{{ item.analysis_text }}</div>
      </div>
    </div>

    <div v-if="store.questions.length > 0 && !store.isParsing" class="import-section">
      <div class="import-section-title">确认导入</div>
      <t-radio-group v-model="importStatus" variant="default-filled">
        <t-radio-button value="draft">草稿</t-radio-button>
        <t-radio-button value="reviewed">已审核</t-radio-button>
      </t-radio-group>
      <t-button theme="primary" :loading="store.isImporting" @click="handleImport" style="margin-left: 12px">
        导入 {{ store.questions.length }} 题
      </t-button>
    </div>

    <t-dialog v-model:visible="editVisible" header="编辑题目" width="600px" :confirm-btn="null" attach="body" :z-index="3000">
      <t-form v-if="editingItem" label-align="top">
        <t-form-item label="题型">
          <t-select v-model="editingItem.question_type" style="width: 100%">
            <t-option v-for="qt in questionTypes" :key="qt.value" :value="qt.value" :label="qt.label" />
          </t-select>
        </t-form-item>
        <t-form-item label="题干">
          <t-textarea v-model="editingItem.stem_text" :autosize="{ minRows: 2, maxRows: 6 }" />
        </t-form-item>
        <t-form-item label="答案">
          <t-textarea v-model="editingItem.answer_text" :autosize="{ minRows: 1, maxRows: 4 }" />
        </t-form-item>
        <t-form-item label="解析">
          <t-textarea v-model="editingItem.analysis_text" :autosize="{ minRows: 1, maxRows: 4 }" />
        </t-form-item>
        <t-form-item label="难度">
          <t-select v-model="editingItem.difficulty" style="width: 120px">
            <t-option value="easy" label="简单" />
            <t-option value="medium" label="中等" />
            <t-option value="hard" label="困难" />
          </t-select>
        </t-form-item>
      </t-form>
      <template #footer>
        <t-button variant="outline" @click="editVisible = false">取消</t-button>
        <t-button theme="primary" @click="saveEditedItem">保存</t-button>
      </template>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useImportWorkbenchStore } from '@/stores/importWorkbench'
import { importQuestions, type ImportQuestionItem, type QuestionType } from '@/api/question'
import { parseImportedBlocks } from '@/api/question_block'
import { deleteDraft } from '@/utils/importDraftDB'

const store = useImportWorkbenchStore()
const importStatus = ref<'draft' | 'reviewed'>('draft')
const editVisible = ref(false)
const editingIndex = ref(-1)
const editingItem = ref<ImportQuestionItem | null>(null)
const emit = defineEmits<{ changed: []; imported: [] }>()

const questionTypes = [
  { value: 'single_choice', label: '单选' },
  { value: 'multiple_choice', label: '多选' },
  { value: 'true_false', label: '判断' },
  { value: 'fill_blank', label: '填空' },
  { value: 'short_answer', label: '简答' },
  { value: 'essay', label: '论述' },
  { value: 'composite', label: '复合' },
]

function questionTypeLabel(t2: QuestionType | string) {
  const map: Record<string, string> = {
    single_choice: '单选', multiple_choice: '多选', true_false: '判断',
    fill_blank: '填空', short_answer: '简答', essay: '论述', composite: '复合',
  }
  return map[t2] || t2
}

function difficultyLabel(d: string) {
  const map: Record<string, string> = { easy: '简单', medium: '中等', hard: '困难' }
  return map[d] || d
}

function editItem(index: number) {
  const item = store.questions[index]
  if (!item) return
  editingIndex.value = index
  editingItem.value = { ...item }
  editVisible.value = true
}

function saveEditedItem() {
  if (editingIndex.value < 0 || !editingItem.value) return
  store.questions[editingIndex.value] = { ...editingItem.value }
  editVisible.value = false
  editingItem.value = null
  editingIndex.value = -1
  emit('changed')
}

function removeItem(index: number) {
  store.questions.splice(index, 1)
  store.questionStats.detected_questions = store.questions.length
  emit('changed')
}

async function handleImport() {
  if (!store.questions.length) {
    MessagePlugin.warning('没有可导入的题目')
    return
  }

  store.isImporting = true
  try {
    const itemsWithStatus = store.questions.map(item => ({
      ...item,
      status: importStatus.value,
    }))
    const result: any = await importQuestions(store.kbId, store.setId, { items: itemsWithStatus })
    const created = result?.created ?? 0
    const errors = Array.isArray(result?.errors) ? result.errors : []

    if (errors.length === 0) {
      MessagePlugin.success(`成功导入 ${created} 题`)
      await deleteDraft(store.kbId, store.setId)
      store.reset()
      emit('imported')
    } else {
      MessagePlugin.warning(`导入 ${created}/${store.questions.length} 题，${errors.length} 条错误。请修复后重试。`)
      store.questionErrors = errors.map((error: any, index: number) => ({
        line_number: Number(error?.line_number ?? index + 1),
        message: String(error?.message ?? error ?? '导入失败'),
      }))
      emit('changed')
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '导入失败')
  } finally {
    store.isImporting = false
  }
}

defineExpose({
  async parseQuestions() {
    if (store.blocks.length === 0) {
      MessagePlugin.warning('请先完成 block review')
      return
    }
    store.isParsing = true
    try {
      const result = await parseImportedBlocks(store.kbId, store.setId, {
        blocks: store.blocks,
        default_difficulty: store.defaultDifficulty,
        strategy_preset: store.strategyPreset,
      })
      store.questions = result.items ?? []
      store.questionErrors = result.errors ?? []
      store.questionWarnings = result.warnings ?? []
      store.questionStats = result.stats ?? { detected_questions: store.questions.length, with_answer: 0, without_answer: 0 }
      emit('changed')
    } catch (e: any) {
      MessagePlugin.error(e?.message || '解析失败')
    } finally {
      store.isParsing = false
    }
  },
})
</script>

<style scoped>
.question-review-panel { padding: 12px 0; }
.stats-bar { margin-bottom: 12px; }
.warnings-box, .errors-box { margin-bottom: 8px; }
.parsing-state { display: flex; justify-content: center; padding: 40px; }
.empty-state { display: flex; justify-content: center; padding: 60px; }
.question-list { max-height: calc(100vh - 320px); overflow-y: auto; }
.question-item { border: 1px solid var(--td-component-stroke); border-radius: 6px; padding: 12px; margin-bottom: 8px; }
.question-item-header { display: flex; align-items: center; gap: 6px; margin-bottom: 6px; flex-wrap: wrap; }
.question-tags { display: flex; gap: 2px; }
.question-stem { font-size: 14px; font-weight: 500; margin-bottom: 4px; line-height: 1.5; }
.question-answer { font-size: 13px; color: var(--td-success-color); margin-bottom: 2px; }
.question-answer .answer-label { font-weight: 500; }
.question-analysis { font-size: 13px; color: var(--td-text-color-secondary); }
.question-analysis .analysis-label { font-weight: 500; }
.import-section { margin-top: 16px; padding: 12px; background: var(--td-bg-color-secondarycontainer); border-radius: 6px; display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.import-section-title { font-weight: 500; }
</style>
