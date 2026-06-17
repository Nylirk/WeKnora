<template>
  <t-dialog v-model:visible="dialogVisible" :header="$t('questionBank.importTitle', '导入题目')" :width="600" @confirm="doImport" @close="dialogVisible = false">
    <t-form label-align="top">
      <t-form-item :label="$t('questionBank.pasteJsonl', '粘贴 JSON 或 JSONL 数据')">
        <t-textarea v-model="rawData" :autosize="{ minRows: 6, maxRows: 20 }" placeholder='[{"question_type":"single_choice","stem_text":"...","answer_text":"..."}]  或每行一个JSON对象' />
      </t-form-item>
    </t-form>
    <div v-if="parseErrors.length" class="import-errors">
      <p class="error-title">解析错误：</p>
      <t-list size="small">
        <t-list-item v-for="(e, i) in parseErrors" :key="i">
          <span class="error-line">第{{ e.line_number }}行: {{ e.message }}</span>
        </t-list-item>
      </t-list>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { importQuestions, type ImportQuestionError, type ImportQuestionItem } from '@/api/question'

const props = defineProps<{ visible: boolean; setId: string }>()
const emit = defineEmits<{ 'update:visible': [v: boolean]; imported: [] }>()

const dialogVisible = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
})
const rawData = ref('')
const parseErrors = ref<ImportQuestionError[]>([])

async function doImport() {
  parseErrors.value = []
  const items = parseInput()
  if (items.length === 0) {
    MessagePlugin.warning('没有可导入的题目')
    return
  }
  try {
    const result = await importQuestions(props.setId, { items })
    if (result.errors && result.errors.length > 0) {
      parseErrors.value = result.errors
    }
    MessagePlugin.success(`成功导入 ${result.created} 道题目`)
    if (result.errors.length === 0) {
      dialogVisible.value = false
    }
    emit('imported')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '导入失败')
  }
}

function parseInput(): ImportQuestionItem[] {
  const text = rawData.value.trim()
  if (!text) return []
  try {
    const arr = JSON.parse(text)
    if (Array.isArray(arr)) {
      return arr.map((item: any, i: number) => normalizeItem(item, i + 1))
    }
    return [normalizeItem(arr, 1)]
  } catch {
    // try JSONL
    const lines = text.split('\n').filter(l => l.trim())
    return lines.map((line, i) => {
      try {
        return normalizeItem(JSON.parse(line), i + 1)
      } catch {
        parseErrors.value.push({ line_number: i + 1, message: 'JSON 解析失败' })
        return null as any
      }
    }).filter(Boolean)
  }
}

function normalizeItem(raw: any, lineNumber: number): ImportQuestionItem {
  return {
    line_number: lineNumber,
    question_type: raw.question_type || 'single_choice',
    stem_text: raw.stem_text || '',
    question_body: raw.question_body || {},
    answer_text: raw.answer_text || '',
    answer_body: raw.answer_body || {},
    analysis_text: raw.analysis_text || '',
    grading_rubric: raw.grading_rubric || {},
    difficulty: raw.difficulty || 'medium',
    knowledge_points: raw.knowledge_points || [],
    tags: raw.tags || [],
    source_knowledge_id: raw.source_knowledge_id || '',
    evidence_chunk_ids: raw.evidence_chunk_ids || [],
  }
}
</script>

<style scoped>
.import-errors { margin-top: 12px; }
.error-title { color: var(--td-error-color); font-weight: 600; }
.error-line { color: var(--td-error-color); }
</style>