<template>
  <div class="workbench-page">
    <!-- Header -->
    <div class="workbench-header">
      <t-button variant="text" @click="handleBack">
        <t-icon name="chevron-left" /> 返回
      </t-button>
      <h3 class="workbench-title">题库导入工作台</h3>
      <div class="header-steps">
        <t-steps :current="store.currentStep === 'block-review' ? 0 : 1" size="small" style="width: 280px">
          <t-step-item title="Block Review" />
          <t-step-item title="Question Review" />
        </t-steps>
      </div>
      <t-space size="small">
        <t-button variant="outline" @click="handleAbandon">放弃导入</t-button>
        <t-button v-if="store.currentStep === 'block-review'" theme="primary" @click="goToQuestionReview">
          下一步：题目解析
        </t-button>
        <t-button v-else variant="outline" @click="store.goToStep('block-review')">
          返回 Block Review
        </t-button>
      </t-space>
    </div>

    <!-- Summary bar -->
    <div class="summary-bar" v-if="store.summary.total_blocks > 0">
      <t-space size="small">
        <t-tag variant="light">{{ store.summary.total_blocks }} blocks</t-tag>
        <t-tag variant="light">{{ store.summary.question_numbers }} 题号</t-tag>
        <t-tag v-if="store.summary.blocks_with_anomalies > 0" theme="warning" variant="light">
          {{ store.summary.blocks_with_anomalies }} 异常
        </t-tag>
      </t-space>
    </div>

    <!-- Steps -->
    <div class="workbench-body">
      <BlockReviewPanel v-if="store.currentStep === 'block-review'" />
      <QuestionReviewPanel v-else ref="questionReviewRef" />
    </div>
  </div>

  <!-- Abandon confirmation -->
  <t-dialog
    v-model:visible="abandonVisible"
    header="放弃导入"
    :confirm-btn="{ content: '保存草稿', theme: 'primary' }"
    :cancel-btn="{ content: '直接放弃' }"
    @confirm="abandonSaveDraft"
    @cancel="abandonDiscard"
  >
    <p>是否保存当前草稿？草稿将保留 7 天。</p>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { useImportWorkbenchStore } from '@/stores/importWorkbench'
import { loadDraft, deleteDraft, saveDraft, cleanExpiredDrafts } from '@/utils/importDraftDB'
import BlockReviewPanel from './components/BlockReviewPanel.vue'
import QuestionReviewPanel from './components/QuestionReviewPanel.vue'

const route = useRoute()
const router = useRouter()
const store = useImportWorkbenchStore()
const questionReviewRef = ref<InstanceType<typeof QuestionReviewPanel> | null>(null)
const abandonVisible = ref(false)

const kbId = route.params.kbId as string
const setId = route.params.setId as string

onMounted(async () => {
  await cleanExpiredDrafts()

  // If store already has blocks (from dialog → store), skip loading
  if (store.blocks.length > 0 && store.kbId === kbId && store.setId === setId) {
    return
  }

  // Try to load draft from IndexedDB
  const draft = await loadDraft(kbId, setId)
  if (draft) {
    const confirmed = confirm(`发现未完成的草稿（${new Date(draft.timestamp).toLocaleString()}），是否恢复？`)
    if (confirmed) {
      store.kbId = kbId
      store.setId = setId
      store.strategyPreset = draft.strategyPreset
      store.defaultDifficulty = draft.defaultDifficulty
      store.importMode = draft.importMode as 'single' | 'batch'
      store.setBlocksFromResponse(draft.blocks, {
        total_blocks: draft.blocks.length,
        blocks_with_anomalies: draft.blocks.filter(b => b.anomalies.length > 0).length,
        question_numbers: draft.blocks.filter(b => b.question_number != null).length,
        anomaly_breakdown: {},
      })
      store.draftExists = true
      return
    } else {
      await deleteDraft(kbId, setId)
    }
  }

  // No blocks and no draft — go back
  MessagePlugin.warning('没有可用的 blocks，请先上传文件。')
  router.replace(`/platform/knowledge-bases/${kbId}`)
})

async function goToQuestionReview() {
  store.goToStep('question-review')
  await nextTick()
  if (questionReviewRef.value) {
    await questionReviewRef.value.parseQuestions()
  }
}

function handleBack() {
  router.push(`/platform/knowledge-bases/${kbId}`)
}

function handleAbandon() {
  abandonVisible.value = true
}

async function abandonSaveDraft() {
  await saveDraft({
    kbId: store.kbId,
    setId: store.setId,
    blocks: store.blocks,
    strategyPreset: store.strategyPreset,
    defaultDifficulty: store.defaultDifficulty,
    importMode: store.importMode,
    timestamp: Date.now(),
  })
  MessagePlugin.success('草稿已保存（7 天有效）')
  abandonVisible.value = false
  router.push(`/platform/knowledge-bases/${kbId}`)
}

async function abandonDiscard() {
  await deleteDraft(store.kbId, store.setId)
  store.reset()
  abandonVisible.value = false
  router.push(`/platform/knowledge-bases/${kbId}`)
}
</script>

<style scoped>
.workbench-page { display: flex; flex-direction: column; height: 100%; min-height: 100vh; padding: 0; }
.workbench-header { display: flex; align-items: center; gap: 16px; padding: 12px 20px; border-bottom: 1px solid var(--td-component-stroke); background: var(--td-bg-color-container); flex-wrap: wrap; }
.workbench-title { margin: 0; font-size: 16px; font-weight: 600; }
.header-steps { margin-left: auto; }
.summary-bar { padding: 8px 20px; border-bottom: 1px solid var(--td-component-stroke); background: var(--td-bg-color-page); }
.workbench-body { flex: 1; overflow: hidden; padding: 0 20px; }
</style>
