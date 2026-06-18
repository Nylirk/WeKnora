<template>
  <div class="question-bank">
    <aside class="set-sidebar">
      <div class="set-sidebar-header">
        <h3>{{ $t('questionBank.title') }}</h3>
        <t-button size="small" theme="primary" @click="openCreateSetDialog">
          <template #icon><t-icon name="add" /></template>
          {{ $t('questionBank.createSet') }}
        </t-button>
      </div>

      <t-input
        v-model.trim="setSearchKeyword"
        size="small"
        clearable
        :placeholder="$t('questionBank.searchSetPlaceholder')"
      >
        <template #prefix-icon><t-icon name="search" /></template>
      </t-input>

      <div class="set-list-header">
        <span>#</span>
        <span>{{ $t('questionBank.setListName') }}</span>
        <span>{{ $t('questionBank.setListCount') }}</span>
        <span>{{ $t('questionBank.setListOperation') }}</span>
      </div>

      <div class="set-list">
        <div v-if="loadingSets" class="set-list-loading">
          <t-loading size="small" />
        </div>
        <div v-else-if="!filteredQuestionSets.length" class="set-list-empty">
          {{ setSearchKeyword ? $t('questionBank.noMatchingSet') : $t('questionBank.noSet') }}
        </div>
        <template v-else>
          <div
            v-for="(set, index) in filteredQuestionSets"
            :key="set.id"
            class="set-list-item"
            :class="{ active: selectedSetId === set.id }"
            role="button"
            tabindex="0"
            @click="openSet(set)"
            @keydown.enter="openSet(set)"
          >
            <span class="set-index">{{ index + 1 }}</span>
            <span class="set-name" :title="set.name">{{ set.name }}</span>
            <span class="set-count">{{ set.question_count || 0 }} 题</span>
            <div class="set-more" @click.stop>
              <t-popup trigger="click" placement="top-right" overlay-class-name="question-set-more-popup">
                <button type="button" class="set-more-btn" :aria-label="$t('questionBank.setListOperation')">
                  <t-icon name="more" size="16px" />
                </button>
                <template #content>
                  <div class="set-menu">
                    <button type="button" class="set-menu-item" @click="openRenameDialog(set)">
                      <t-icon name="edit" />
                      <span>{{ $t('questionBank.renameSet') }}</span>
                    </button>
                    <button type="button" class="set-menu-item danger" @click="confirmDeleteSet(set)">
                      <t-icon name="delete" />
                      <span>{{ $t('common.delete') }}</span>
                    </button>
                  </div>
                </template>
              </t-popup>
            </div>
          </div>
        </template>
      </div>
    </aside>

    <section class="question-content">
      <QuestionSetDetail
        v-if="selectedSetId"
        :key="selectedSetId"
        :set-id="selectedSetId"
        :set-name="selectedSet?.name"
        :knowledge-base-id="knowledgeBaseId"
        @generated="loadSets"
        @changed="handleDetailChanged"
      />
      <div v-else-if="!loadingSets" class="question-bank-empty">
        {{ $t('questionBank.noSetDescription') }}
      </div>
    </section>

    <t-dialog
      v-model:visible="createSetVisible"
      :header="$t('questionBank.createSet')"
      :confirm-btn="{ content: $t('common.confirm'), loading: creatingSet }"
      :cancel-btn="{ content: $t('common.cancel'), disabled: creatingSet }"
      @confirm="createSet"
    >
      <t-form>
        <t-form-item :label="$t('questionBank.setName')">
          <t-input v-model="newSetName" :placeholder="$t('questionBank.setNamePlaceholder')" />
        </t-form-item>
        <t-form-item :label="$t('questionBank.description')">
          <t-textarea v-model="newSetDescription" :placeholder="$t('questionBank.descPlaceholder')" />
        </t-form-item>
      </t-form>
    </t-dialog>

    <t-dialog
      v-model:visible="renameVisible"
      :header="$t('questionBank.renameSetTitle')"
      :confirm-btn="{ content: $t('common.confirm'), loading: renamingSet }"
      :cancel-btn="{ content: $t('common.cancel'), disabled: renamingSet }"
      @confirm="renameSet"
    >
      <t-form>
        <t-form-item :label="$t('questionBank.setName')">
          <t-input v-model="renameSetName" :placeholder="$t('questionBank.setNamePlaceholder')" />
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import {
  createQuestionSet as apiCreateSet,
  deleteQuestionSet as apiDeleteSet,
  listQuestionSets,
  updateQuestionSet as apiUpdateSet,
  type QuestionSet,
} from '@/api/question'
import QuestionSetDetail from './QuestionSetDetail.vue'

const props = defineProps<{
  knowledgeBaseId: string
  enabled: boolean
}>()

const questionSets = ref<QuestionSet[]>([])
const loadingSets = ref(false)
const selectedSetId = ref('')
const setSearchKeyword = ref('')
const createSetVisible = ref(false)
const creatingSet = ref(false)
const newSetName = ref('')
const newSetDescription = ref('')
const renameVisible = ref(false)
const renamingSet = ref(false)
const renameTarget = ref<QuestionSet | null>(null)
const renameSetName = ref('')
let loadRequestId = 0

const filteredQuestionSets = computed(() => {
  const keyword = setSearchKeyword.value.trim().toLowerCase()
  if (!keyword) return questionSets.value
  return questionSets.value.filter(set => (set.name || '').toLowerCase().includes(keyword))
})
const selectedSet = computed(() => questionSets.value.find(set => set.id === selectedSetId.value))

async function loadSets() {
  const requestId = ++loadRequestId
  const knowledgeBaseId = props.knowledgeBaseId
  if (!props.enabled || !knowledgeBaseId) return

  loadingSets.value = true
  try {
    const result = await listQuestionSets(knowledgeBaseId, 1, 200)
    if (requestId !== loadRequestId || !props.enabled || props.knowledgeBaseId !== knowledgeBaseId) return
    const data = (result as any)?.data ?? result
    questionSets.value = Array.isArray(data) ? data : []
    const selectionStillExists = questionSets.value.some(set => set.id === selectedSetId.value)
    if (!selectionStillExists) selectedSetId.value = questionSets.value[0]?.id || ''
  } catch (e: any) {
    if (requestId === loadRequestId && props.enabled && props.knowledgeBaseId === knowledgeBaseId) {
      MessagePlugin.error(e?.message || '加载题集列表失败')
    }
  } finally {
    if (requestId === loadRequestId) loadingSets.value = false
  }
}

function openCreateSetDialog() {
  newSetName.value = ''
  newSetDescription.value = ''
  createSetVisible.value = true
}

async function createSet() {
  if (!props.enabled || !props.knowledgeBaseId || creatingSet.value) return
  if (!newSetName.value.trim()) {
    MessagePlugin.warning('请输入题集名称')
    return
  }
  creatingSet.value = true
  try {
    const result: any = await apiCreateSet(props.knowledgeBaseId, {
      name: newSetName.value.trim(),
      description: newSetDescription.value.trim(),
    })
    const created: QuestionSet = result?.data ?? result
    createSetVisible.value = false
    setSearchKeyword.value = ''
    await loadSets()
    if (created?.id) {
      if (!questionSets.value.some(set => set.id === created.id)) questionSets.value.unshift(created)
      selectedSetId.value = created.id
    }
    MessagePlugin.success('题集创建成功')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '创建题集失败')
  } finally {
    creatingSet.value = false
  }
}

function openSet(row: QuestionSet) {
  selectedSetId.value = row.id
}

function updateQuestionCount(setId: string, total: number) {
  const questionSet = questionSets.value.find(set => set.id === setId)
  if (questionSet) questionSet.question_count = total
}

async function handleDetailChanged(total: number) {
  const setId = selectedSetId.value
  if (!setId) return
  updateQuestionCount(setId, total)
  await loadSets()
  if (selectedSetId.value === setId) updateQuestionCount(setId, total)
}

function openRenameDialog(row: QuestionSet) {
  renameTarget.value = row
  renameSetName.value = row.name
  renameVisible.value = true
}

async function renameSet() {
  const target = renameTarget.value
  const name = renameSetName.value.trim()
  if (!target || renamingSet.value) return
  if (!name) {
    MessagePlugin.warning('请输入题集名称')
    return
  }
  if (name === target.name) {
    renameVisible.value = false
    return
  }
  renamingSet.value = true
  try {
    await apiUpdateSet(props.knowledgeBaseId, target.id, { name })
    renameVisible.value = false
    MessagePlugin.success('重命名成功')
    await loadSets()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重命名失败')
  } finally {
    renamingSet.value = false
  }
}

function selectAfterCurrentSetDeleted() {
  if (filteredQuestionSets.value.length > 0) {
    selectedSetId.value = filteredQuestionSets.value[0].id
    return
  }
  if (questionSets.value.length > 0) {
    setSearchKeyword.value = ''
    selectedSetId.value = questionSets.value[0].id
    return
  }
  selectedSetId.value = ''
}

function confirmDeleteSet(row: QuestionSet) {
  if (!props.enabled || !props.knowledgeBaseId) return
  const deletingCurrent = selectedSetId.value === row.id
  const dialog = DialogPlugin.confirm({
    header: '删除题集',
    body: `确认删除题集「${row.name}」？删除后将同时删除该题集下的题目。`,
    confirmBtn: { content: '删除', theme: 'danger' },
    cancelBtn: '取消',
    onConfirm: async () => {
      try {
        await apiDeleteSet(props.knowledgeBaseId, row.id)
        await loadSets()
        if (deletingCurrent) selectAfterCurrentSetDeleted()
        MessagePlugin.success('删除成功')
        dialog.hide()
      } catch (e: any) {
        MessagePlugin.error(e?.message || '删除失败')
      }
    },
  })
}

watch(
  () => [props.knowledgeBaseId, props.enabled] as const,
  async ([id, enabled]) => {
    loadRequestId += 1
    selectedSetId.value = ''
    questionSets.value = []
    setSearchKeyword.value = ''
    loadingSets.value = false
    if (!id || !enabled) return
    await loadSets()
  },
  { immediate: true },
)
</script>

<style scoped>
.question-bank { display: flex; flex: 1; min-height: 0; border-top: 1px solid var(--td-component-stroke); }
.set-sidebar { width: 280px; flex-shrink: 0; padding: 16px 16px 0 0; border-right: 1px solid var(--td-component-stroke); display: flex; flex-direction: column; gap: 12px; }
.set-sidebar-header { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.set-sidebar-header h3 { margin: 0; font-size: 16px; }
.set-list-header,
.set-list-item { display: grid; grid-template-columns: 24px minmax(0, 1fr) 58px 32px; align-items: center; column-gap: 6px; }
.set-list-header { padding: 0 8px; color: var(--td-text-color-placeholder); font-size: 12px; }
.set-list { flex: 1; min-height: 0; overflow-y: auto; }
.set-list-loading,
.set-list-empty { padding: 28px 8px; color: var(--td-text-color-placeholder); text-align: center; font-size: 13px; }
.set-list-item { min-height: 40px; padding: 0 8px; border-radius: 6px; color: var(--td-text-color-primary); cursor: pointer; }
.set-list-item:hover { background: var(--td-bg-color-container-hover); }
.set-list-item.active { background: var(--td-brand-color-light); color: var(--td-brand-color); }
.set-index,
.set-count { color: var(--td-text-color-secondary); font-size: 12px; }
.set-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
.set-more { display: flex; justify-content: center; }
.set-more-btn { display: flex; align-items: center; justify-content: center; width: 28px; height: 28px; padding: 0; border: 0; border-radius: 4px; color: inherit; background: transparent; cursor: pointer; }
.set-more-btn:hover { background: var(--td-bg-color-container-active); }
.set-menu { min-width: 112px; padding: 4px; }
.set-menu-item { width: 100%; display: flex; align-items: center; gap: 8px; padding: 7px 10px; border: 0; border-radius: 4px; color: var(--td-text-color-primary); background: transparent; cursor: pointer; text-align: left; }
.set-menu-item:hover { background: var(--td-bg-color-container-hover); }
.set-menu-item.danger { color: var(--td-error-color); }
.question-content { flex: 1; min-width: 0; min-height: 0; overflow: auto; padding: 16px 0 0 20px; }
.question-bank-empty { height: 100%; min-height: 240px; display: flex; align-items: center; justify-content: center; color: var(--td-text-color-placeholder); }
</style>
