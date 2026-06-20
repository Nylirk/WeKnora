<template>
  <div class="question-bank">
    <aside class="tag-sidebar">
      <div class="sidebar-header">
        <div class="sidebar-title">
          <span>题目分类</span>
          <span class="sidebar-count">({{ questionSets.length }})</span>
        </div>
        <div class="sidebar-actions">
          <t-button
            v-if="!creatingInlineSet"
            size="small"
            variant="text"
            class="create-tag-btn"
            :aria-label="'创建分类'"
            :title="'创建分类'"
            @click="startCreateSet"
          >
            <t-icon name="add" />
          </t-button>
        </div>
      </div>
      <div class="tag-search-bar">
        <t-input v-model.trim="setSearchKeyword" size="small"
          :placeholder="'搜索分类'" clearable>
          <template #prefix-icon>
            <t-icon name="search" size="14px" />
          </template>
        </t-input>
      </div>
      <div class="tag-list">
        <div v-if="loadingSets" v-for="n in 6" :key="'skel-' + n" class="tag-list-item"
          style="cursor: default; pointer-events: none;">
          <div class="tag-list-left" style="gap: 12px; width: 100%;">
            <t-skeleton animation="gradient" :row-col="[{ width: '80%', height: '18px' }]" />
          </div>
        </div>
        <template v-else>
          <!-- Inline create row -->
          <div v-if="creatingInlineSet" class="tag-list-item tag-editing" @click.stop>
            <div class="tag-list-left">
              <span class="tag-hash-icon">#</span>
              <div class="tag-edit-input">
                <t-input ref="newSetNameRef" v-model="newInlineSetName" size="small" :maxlength="40"
                  :placeholder="'请输入分类名称'"
                  @enter="confirmCreateSet"
                  @keydown="(_v: any, ctx: any) => { if (ctx?.e?.key === 'Escape') { ctx.e.stopPropagation(); ctx.e.preventDefault(); cancelCreateSet() } }" />
              </div>
            </div>
            <div class="tag-inline-actions">
              <t-button variant="text" theme="default" size="small" class="tag-action-btn confirm"
                :loading="creatingInlineSetLoading" @click.stop="confirmCreateSet">
                <t-icon name="check" size="16px" />
              </t-button>
              <t-button variant="text" theme="default" size="small" class="tag-action-btn cancel"
                @click.stop="cancelCreateSet">
                <t-icon name="close" size="16px" />
              </t-button>
            </div>
          </div>

          <div v-if="!creatingInlineSet && !filteredQuestionSets.length && !loadingSets" class="tag-list-item"
            style="cursor: default; color: var(--td-text-color-placeholder); justify-content: center;">
            {{ setSearchKeyword ? '未找到匹配的分类' : '暂无分类' }}
          </div>

          <template v-if="filteredQuestionSets.length">
            <div
              v-for="set in filteredQuestionSets"
              :key="set.id"
              class="tag-list-item"
              :class="{ active: selectedSetId === set.id, editing: editingSetId === set.id }"
              @click="editingSetId ? undefined : openSet(set)"
            >
              <div class="tag-list-left">
                <span class="tag-hash-icon">#</span>
                <template v-if="editingSetId === set.id">
                  <div class="tag-edit-input" @click.stop>
                    <t-input :ref="setEditingSetInputRef(set.id)" v-model="editSetName" size="small"
                      :maxlength="40" @enter="submitEditSet"
                      @keydown="(_v: any, ctx: any) => { if (ctx?.e?.key === 'Escape') { ctx.e.stopPropagation(); ctx.e.preventDefault(); cancelEditSet() } }" />
                  </div>
                </template>
                <template v-else>
                  <span class="tag-name" :title="set.name">{{ set.name }}</span>
                </template>
              </div>
              <div class="tag-list-right">
                <span class="tag-count">{{ set.question_count || 0 }}</span>
                <div v-if="editingSetId !== set.id" class="tag-more" @click.stop>
                  <t-popup trigger="click" placement="top-right" overlay-class-name="question-set-more-popup">
                    <button type="button" class="tag-more-btn" :aria-label="'更多操作'">
                      <t-icon name="more" size="14px" />
                    </button>
                    <template #content>
                      <div class="tag-menu">
                        <button type="button" class="tag-menu-item" @click="startEditSet(set)">
                          <t-icon name="edit" />
                          <span>重命名分类</span>
                        </button>
                        <button type="button" class="tag-menu-item danger" @click="confirmDeleteSet(set)">
                          <t-icon name="delete" />
                          <span>删除分类</span>
                        </button>
                      </div>
                    </template>
                  </t-popup>
                </div>
              </div>
            </div>
          </template>
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
        请选择左侧分类查看题目
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import {
  createQuestionSet as apiCreateSet,
  deleteQuestionSet as apiDeleteSet,
  listQuestionSets,
  updateQuestionSet as apiUpdateSet,
  type QuestionSet,
} from '@/api/question'
import type { ComponentPublicInstance } from 'vue'
import QuestionSetDetail from './QuestionSetDetail.vue'

type InputInstance = ComponentPublicInstance<{ focus: () => void; select: () => void }>;

const props = defineProps<{
  knowledgeBaseId: string
  enabled: boolean
}>()

const questionSets = ref<QuestionSet[]>([])
const loadingSets = ref(false)
const selectedSetId = ref('')
const setSearchKeyword = ref('')
let loadRequestId = 0

// Inline create state
const creatingInlineSet = ref(false)
const creatingInlineSetLoading = ref(false)
const newInlineSetName = ref('')
const newSetNameRef = ref<InputInstance | null>(null)

// Inline edit state
const editingSetId = ref<string | null>(null)
const editSetName = ref('')
const editingSetSubmitting = ref(false)
const editingSetInputRefs = new Map<string, InputInstance | null>()
const setEditingSetInputRef = (id: string) => (el: InputInstance | null) => {
  editingSetInputRefs.set(id, el)
}

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
      MessagePlugin.error(e?.message || '加载分类列表失败')
    }
  } finally {
    if (requestId === loadRequestId) loadingSets.value = false
  }
}

// Inline create
function startCreateSet() {
  if (!props.enabled || !props.knowledgeBaseId) return
  if (creatingInlineSet.value) return
  editingSetId.value = null
  editSetName.value = ''
  creatingInlineSet.value = true
  newInlineSetName.value = ''
  nextTick(() => {
    newSetNameRef.value?.focus?.()
    newSetNameRef.value?.select?.()
  })
}

function cancelCreateSet() {
  creatingInlineSet.value = false
  newInlineSetName.value = ''
}

async function confirmCreateSet() {
  if (!props.enabled || !props.knowledgeBaseId || creatingInlineSetLoading.value) return
  const name = newInlineSetName.value.trim()
  if (!name) {
    MessagePlugin.warning('分类名称不能为空')
    return
  }
  if ([...name].length > 40) {
    MessagePlugin.warning('分类名称不能超过 40 个字符')
    return
  }
  // Check for duplicate name in current list
  if (questionSets.value.some(s => s.name === name)) {
    MessagePlugin.warning('当前题库中已存在同名分类')
    return
  }
  creatingInlineSetLoading.value = true
  try {
    const result: any = await apiCreateSet(props.knowledgeBaseId, { name })
    const created: QuestionSet = result?.data ?? result
    cancelCreateSet()
    setSearchKeyword.value = ''
    await loadSets()
    if (created?.id) {
      if (!questionSets.value.some(set => set.id === created.id)) questionSets.value.unshift(created)
      selectedSetId.value = created.id
    }
    MessagePlugin.success('分类创建成功')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '创建分类失败')
  } finally {
    creatingInlineSetLoading.value = false
  }
}

// Inline edit
function startEditSet(row: QuestionSet) {
  creatingInlineSet.value = false
  newInlineSetName.value = ''
  editingSetId.value = row.id
  editSetName.value = row.name
  nextTick(() => {
    const ref = editingSetInputRefs.get(row.id)
    ref?.focus?.()
    ref?.select?.()
  })
}

function cancelEditSet() {
  editingSetId.value = null
  editSetName.value = ''
}

async function submitEditSet() {
  const id = editingSetId.value
  const name = editSetName.value.trim()
  if (!id || editingSetSubmitting.value) return
  if (!name) {
    MessagePlugin.warning('分类名称不能为空')
    return
  }
  if ([...name].length > 40) {
    MessagePlugin.warning('分类名称不能超过 40 个字符')
    return
  }
  const target = questionSets.value.find(s => s.id === id)
  if (target && name === target.name) {
    cancelEditSet()
    return
  }
  editingSetSubmitting.value = true
  try {
    await apiUpdateSet(props.knowledgeBaseId, id, { name })
    cancelEditSet()
    MessagePlugin.success('重命名成功')
    await loadSets()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重命名失败')
  } finally {
    editingSetSubmitting.value = false
  }
}

function openSet(row: QuestionSet) {
  if (editingSetId.value) return
  selectedSetId.value = row.id
}

function updateQuestionCount(setId: string, total: number) {
  const qs = questionSets.value.find(set => set.id === setId)
  if (qs) qs.question_count = total
}

async function handleDetailChanged(total: number) {
  const setId = selectedSetId.value
  if (!setId) return
  updateQuestionCount(setId, total)
  await loadSets()
  if (selectedSetId.value === setId) updateQuestionCount(setId, total)
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
    header: '删除分类',
    body: `确认删除分类「${row.name}」？删除后将同时删除该分类下的所有题目。`,
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
    creatingInlineSet.value = false
    newInlineSetName.value = ''
    editingSetId.value = null
    if (!id || !enabled) return
    await loadSets()
  },
  { immediate: true },
)
</script>

<style scoped>
.question-bank { display: flex; flex: 1; min-height: 0; border-top: 1px solid var(--td-component-stroke); }

/* Reuse KnowledgeBase.vue tag-sidebar styles */
.tag-sidebar {
  width: 180px;
  flex-shrink: 0;
  padding: 16px 12px 0 0;
  border-right: 1px solid var(--td-component-stroke);
  display: flex;
  flex-direction: column;
  max-height: 100%;
  min-height: 0;
}
.sidebar-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; padding: 0 4px; color: var(--td-text-color-primary); }
.sidebar-title { display: flex; align-items: baseline; gap: 6px; font-size: 14px; font-weight: 600; letter-spacing: 0.5px; }
.sidebar-count { font-size: 12px; color: var(--td-text-color-placeholder); font-weight: 400; }
.sidebar-actions { display: flex; gap: 6px; align-items: center; }
.create-tag-btn { width: 24px; height: 24px; padding: 0; border-radius: 4px; display: flex; align-items: center; justify-content: center; color: var(--td-text-color-secondary); transition: all 0.2s ease; }
.create-tag-btn:hover { background: var(--td-bg-color-secondarycontainer); color: var(--td-brand-color); }
.tag-search-bar { margin-bottom: 12px; padding: 0 4px; }
.tag-search-bar :deep(.t-input) { font-size: 13px; background-color: var(--td-bg-color-secondarycontainer); border-color: transparent; border-radius: 6px; box-shadow: none !important; }
.tag-search-bar :deep(.t-input):hover,
.tag-search-bar :deep(.t-input):focus,
.tag-search-bar :deep(.t-input).t-is-focused { border-color: var(--td-brand-color); background-color: var(--td-bg-color-container); box-shadow: none !important; }
.tag-list { display: flex; flex-direction: column; gap: 5px; flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; scrollbar-width: none; }
.tag-list::-webkit-scrollbar { display: none; }
.tag-list-item { display: flex; align-items: center; justify-content: space-between; padding: 8px 8px; border-radius: 6px; color: var(--td-text-color-primary); cursor: pointer; transition: all 0.2s ease; font-size: 13px; }
.tag-list-item:hover { background: var(--td-bg-color-secondarycontainer); }
.tag-list-item.active { background: var(--td-brand-color-light); color: var(--td-brand-color); }
.tag-list-item.active .tag-hash-icon { color: var(--td-brand-color); }
.tag-list-item.active .tag-name { font-weight: 500; }
.tag-list-item.active .tag-count { color: var(--td-brand-color); }
.tag-list-item.editing { background: transparent; border: none; }
.tag-list-item.tag-editing { cursor: default; padding-right: 8px; background: transparent; border: none; }
.tag-list-left { display: flex; align-items: center; gap: 8px; min-width: 0; flex: 1; }
.tag-hash-icon { flex-shrink: 0; color: var(--td-text-color-secondary); font-family: var(--app-font-family-mono); font-size: 16px; font-weight: 500; width: 16px; text-align: center; }
.tag-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; line-height: 1.4; }
.tag-list-right { display: flex; align-items: center; gap: 6px; margin-left: 8px; flex-shrink: 0; }
.tag-count { font-size: 12px; color: var(--td-text-color-placeholder); }
.tag-edit-input { flex: 1; min-width: 0; }
.tag-inline-actions { display: flex; gap: 4px; margin-left: auto; }
.tag-action-btn { width: 24px; height: 24px; padding: 0; }
.tag-action-btn.confirm { color: var(--td-success-color); }
.tag-action-btn.cancel { color: var(--td-text-color-secondary); }
.tag-more { display: flex; align-items: center; }
.tag-more-btn { display: flex; align-items: center; justify-content: center; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 4px; color: inherit; background: transparent; cursor: pointer; }
.tag-more-btn:hover { background: var(--td-bg-color-container-active); }
.tag-menu { min-width: 96px; padding: 4px; }
.tag-menu-item { width: 100%; display: flex; align-items: center; gap: 8px; padding: 7px 10px; border: 0; border-radius: 4px; color: var(--td-text-color-primary); background: transparent; cursor: pointer; text-align: left; }
.tag-menu-item:hover { background: var(--td-bg-color-container-hover); }
.tag-menu-item.danger { color: var(--td-error-color); }

.question-content { flex: 1; min-width: 0; min-height: 0; overflow: auto; padding: 16px 0 0 20px; }
.question-bank-empty { height: 100%; min-height: 240px; display: flex; align-items: center; justify-content: center; color: var(--td-text-color-placeholder); }
</style>
