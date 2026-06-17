<template>
  <div class="question-bank">
    <template v-if="!selectedSetId">
      <div class="question-bank-header">
        <div>
          <h2>{{ $t('questionBank.title', '题集') }}</h2>
          <p>{{ $t('questionBank.description', '管理题集与题目，导入题目，导出评测集。') }}</p>
        </div>
        <t-button theme="primary" @click="openCreateSetDialog">
          <template #icon><t-icon name="add" /></template>
          {{ $t('questionBank.createSet', '创建题集') }}
        </t-button>
      </div>

      <t-table
        :data="questionSets"
        :loading="loadingSets"
        row-key="id"
        hover
        @row-click="openSet"
      >
        <t-table-column :title="$t('questionBank.setName', '题集名称')" prop="name" />
        <t-table-column :title="$t('questionBank.sourceType', '来源')" prop="source_type" :width="100">
          <template #default="{ row }">
            <t-tag :theme="sourceTypeTheme(row.source_type)" size="small">
              {{ sourceTypeLabel(row.source_type) }}
            </t-tag>
          </template>
        </t-table-column>
        <t-table-column :title="$t('questionBank.questionCount', '题目数')" prop="question_count" :width="100" />
        <t-table-column :title="$t('questionBank.status', '状态')" prop="status" :width="100">
          <template #default="{ row }">
            <t-tag :theme="setStatusTheme(row.status)" size="small">
              {{ setStatusLabel(row.status) }}
            </t-tag>
          </template>
        </t-table-column>
        <t-table-column :title="$t('questionBank.createdAt', '创建时间')" prop="created_at" :width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </t-table-column>
        <t-table-column :title="$t('common.action', '操作')" :width="160" fixed="right">
          <template #default="{ row }">
            <t-link theme="primary" @click.stop="openSet(row)">{{ $t('common.view', '查看') }}</t-link>
            <t-link theme="danger" @click.stop="deleteSet(row)">{{ $t('common.delete', '删除') }}</t-link>
          </template>
        </t-table-column>
      </t-table>

      <t-dialog
        v-model:visible="createSetVisible"
        :header="$t('questionBank.createSet', '创建题库')"
        @confirm="createSet"
      >
        <t-form>
          <t-form-item :label="$t('questionBank.setName', '题库名称')">
            <t-input v-model="newSetName" :placeholder="$t('questionBank.setNamePlaceholder', '请输入题库名称')" />
          </t-form-item>
          <t-form-item :label="$t('questionBank.description', '描述')">
            <t-textarea v-model="newSetDescription" :placeholder="$t('questionBank.descPlaceholder', '可选描述')" />
          </t-form-item>
        </t-form>
      </t-dialog>
    </template>

    <template v-else>
      <QuestionSetDetail
        :set-id="selectedSetId"
        :knowledge-base-id="knowledgeBaseId"
        @back="selectedSetId = ''"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listQuestionSets, createQuestionSet as apiCreateSet,
  deleteQuestionSet as apiDeleteSet,
  type QuestionSet,
} from '@/api/question'

const props = defineProps<{
  knowledgeBaseId: string
}>()

const questionSets = ref<QuestionSet[]>([])
const loadingSets = ref(false)
const selectedSetId = ref('')
const createSetVisible = ref(false)
const newSetName = ref('')
const newSetDescription = ref('')

async function loadSets() {
  loadingSets.value = true
  try {
    const res = await listQuestionSets(props.knowledgeBaseId, 1, 200)
    questionSets.value = res.data || []
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载题库列表失败')
  } finally {
    loadingSets.value = false
  }
}

function openCreateSetDialog() {
  newSetName.value = ''
  newSetDescription.value = ''
  createSetVisible.value = true
}

async function createSet() {
  if (!newSetName.value.trim()) {
    MessagePlugin.warning('请输入题库名称')
    return
  }
  try {
    await apiCreateSet(props.knowledgeBaseId, {
      name: newSetName.value.trim(),
      description: newSetDescription.value.trim(),
    })
    createSetVisible.value = false
    MessagePlugin.success('题库创建成功')
    await loadSets()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '创建题库失败')
  }
}

function openSet(row: QuestionSet) {
  selectedSetId.value = row.id
}

async function deleteSet(row: QuestionSet) {
  try {
    await apiDeleteSet(props.knowledgeBaseId, row.id)
    MessagePlugin.success('删除成功')
    await loadSets()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

function sourceTypeLabel(t: string) {
  const map: Record<string, string> = { manual: '手动', 'import': '导入', generated: '生成', exam_paper: '试卷' }
  return map[t] || t
}
function sourceTypeTheme(t: string) {
  const map: Record<string, string> = { manual: 'default', 'import': 'primary', generated: 'warning', exam_paper: 'primary' }
  return map[t] || 'default'
}
function setStatusLabel(s: string) {
  const map: Record<string, string> = { active: '启用', completed: '完成', pending: '待定', failed: '失败' }
  return map[s] || s
}
function setStatusTheme(s: string) {
  const map: Record<string, string> = { active: 'success', completed: 'primary', pending: 'warning', failed: 'danger' }
  return map[s] || 'default'
}
function formatDate(d: string) {
  return d ? new Date(d).toLocaleString() : ''
}

onMounted(loadSets)
watch(() => props.knowledgeBaseId, loadSets)

import QuestionSetDetail from './QuestionSetDetail.vue'
</script>

<style scoped>
.question-bank { padding: 16px; }
.question-bank-header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; }
.question-bank-header h2 { margin: 0 0 4px; }
.question-bank-header p { margin: 0; color: var(--td-text-color-secondary); font-size: 14px; }
</style>