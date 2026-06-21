import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import zhCN from '../../i18n/locales/zh-CN.ts'

const source = readFileSync(new URL('./components/QuestionSetDetail.vue', import.meta.url), 'utf8')
const bankSource = readFileSync(new URL('./components/QuestionBank.vue', import.meta.url), 'utf8')
const importSource = readFileSync(new URL('./components/QuestionImportDialog.vue', import.meta.url), 'utf8')
const fileImportSource = readFileSync(
  new URL('./components/QuestionFileImportDialog.vue', import.meta.url),
  'utf8',
)
const questionApiSource = readFileSync(
  new URL('../../api/question.ts', import.meta.url),
  'utf8',
)
const workbenchSource = readFileSync(new URL('./QuestionImportWorkbench.vue', import.meta.url), 'utf8')
const blockReviewSource = readFileSync(new URL('./components/BlockReviewPanel.vue', import.meta.url), 'utf8')
const questionReviewSource = readFileSync(new URL('./components/QuestionReviewPanel.vue', import.meta.url), 'utf8')
const routerSource = readFileSync(new URL('../../router/index.ts', import.meta.url), 'utf8')

function sourceSection(content: string, start: string, end: string): string {
  const startIndex = content.indexOf(start)
  const endIndex = content.indexOf(end, startIndex + start.length)
  return startIndex >= 0 && endIndex > startIndex ? content.slice(startIndex, endIndex) : ''
}

test('uses supported TDesign columns and named cell slots', () => {
  assert.equal(source.includes('<t-table-column'), false)
  assert.equal(source.includes(':columns="questionColumns"'), true)
  assert.equal(source.includes('#question_type="{ row }"'), true)
  assert.equal(source.includes('#difficulty="{ row }"'), true)
  assert.equal(source.includes('#status="{ row }"'), true)
  assert.equal(source.includes('#operation="{ row }"'), true)
})

test('does not repeat the question count in the detail header', () => {
  assert.equal(source.includes('question-total'), false)
  // questionTotal exists for pagination state — not for display in header
})

test('renders only spaced edit and delete row actions', () => {
  const operationSlot = source.match(/<template #operation="\{ row \}">([\s\S]*?)<\/template>/)?.[1] || ''

  assert.equal(operationSlot.includes('<t-space size="small">'), true)
  assert.equal(operationSlot.includes('openEditDialog(row)'), true)
  assert.equal(operationSlot.includes('removeQuestion(row)'), true)
  assert.equal(operationSlot.includes('reviewQuestion(row)'), false)
  assert.equal(source.includes('updateQuestionStatus'), true)
})

test('syncs question totals to the selected set', () => {
  assert.equal(source.includes("emit('changed', total)"), true)
  assert.equal(bankSource.includes('@changed="handleDetailChanged"'), true)
  assert.equal(bankSource.includes('set.question_count || 0'), true)
  assert.equal(importSource.includes('allowDuplicates'), true)
})

test('renders the question category sidebar with document-category style markup', () => {
  // Uses tag-sidebar / tag-list / tag-list-item classes matching KnowledgeBase.vue
  assert.equal(bankSource.includes('class="tag-sidebar"'), true)
  assert.equal(bankSource.includes('class="sidebar-header"'), true)
  assert.equal(bankSource.includes('class="tag-search-bar"'), true)
  assert.equal(bankSource.includes('class="tag-list"'), true)
  assert.equal(bankSource.includes('class="tag-list-item"'), true)
  assert.equal(bankSource.includes('class="tag-hash-icon"'), true)
  // Title uses "题目分类"
  assert.equal(bankSource.includes('题目分类'), true)
  // Search placeholder "搜索分类"
  assert.equal(bankSource.includes('搜索分类'), true)
  // Inline create: + button, input, confirm/cancel
  assert.equal(bankSource.includes('creatingInlineSet'), true)
  assert.equal(bankSource.includes('class="tag-edit-input"'), true)
  assert.equal(bankSource.includes('class="tag-inline-actions"'), true)
  assert.equal(bankSource.includes('startCreateSet'), true)
  assert.equal(bankSource.includes('confirmCreateSet'), true)
  assert.equal(bankSource.includes('cancelCreateSet'), true)
  // Count is pure number, no "题" suffix
  assert.equal(bankSource.includes('{{ set.question_count || 0 }} 题'), false)
  assert.equal(bankSource.includes('set.question_count'), true)
  // Inline edit: rename without modal
  assert.equal(bankSource.includes('startEditSet'), true)
  assert.equal(bankSource.includes('submitEditSet'), true)
  assert.equal(bankSource.includes('cancelEditSet'), true)
  // Popup menu for delete
  assert.equal(bankSource.includes('<t-popup'), true)
  assert.equal(bankSource.includes('confirmDeleteSet(set)'), true)
  // No old set-list-header grid
  assert.equal(bankSource.includes('class="set-list-header"'), false)
  assert.equal(bankSource.includes('source_type'), false)
  assert.equal(bankSource.includes('class="set-meta"'), false)
})

test('top-level import menu offers manual, file, and disabled batch import', () => {
  assert.equal(source.includes('openManualImport'), true)
  assert.equal(source.includes('手动导入'), true)
  assert.equal(source.includes('手动创建一道题目'), true)
  assert.equal(source.includes('openSingleImport'), true)
  assert.equal(source.includes('文件导入'), true)
  assert.equal(source.includes('导入一个文件并进入题目审核工作台'), true)
  assert.equal(source.includes('批量导入'), true)
  assert.equal(source.includes('即将支持'), true)
  assert.equal(source.includes('openJsonImport'), false)
  assert.equal(source.includes("openFileImport('word')"), false)
  assert.equal(source.includes("openFileImport('pdf')"), false)
  assert.equal(source.includes('QuestionFileImportDialog'), true)
  assert.equal(source.includes('import-mode="single"'), true)
  assert.equal((source.match(/class="import-type-item" disabled/g) || []).length, 1)
})

test('closes import type menu before opening the single import dialog', () => {
  assert.equal(source.includes('headerImportMenuVisible'), true)
  assert.equal(source.includes('closeAllImportMenus'), true)
  assert.equal(source.includes('await closeAllImportMenus()'), true)
  assert.equal(source.includes('v-model:visible="headerImportMenuVisible"'), true)
  assert.equal(source.includes('loadDraft(props.knowledgeBaseId, props.setId)'), true)
  assert.equal(source.includes('restoreDraftVisible.value = true'), true)
  // A new import destroys the previous dialog instance before opening a fresh session.
  assert.equal(source.includes("fileImportVisible.value = false"), true)
  assert.equal(source.includes("fileImportSession.value += 1"), true)
})

test('uses a compact JSON import dialog with local file parsing', () => {
  assert.equal(importSource.includes('class="format-hint"'), false)
  assert.equal(importSource.includes('class="format-examples"'), false)
  assert.equal(importSource.includes('value="paste"'), true)
  assert.equal(importSource.includes('value="file"'), true)
  assert.equal(importSource.includes('accept=".json,.jsonl,application/json,text/plain"'), true)
  assert.equal(importSource.includes('await file.text()'), true)
  assert.equal(importSource.includes('parseErrorCount'), true)
})

test('file import dialog uses horizontal format cards and top actions', () => {
  assert.equal(fileImportSource.includes(':close-on-overlay-click="false"'), true)
  assert.equal(fileImportSource.includes('导入题目'), true)
  assert.equal(fileImportSource.includes('class="format-card"'), true)
  assert.equal(fileImportSource.includes('class="format-cards"'), true)
  assert.equal(fileImportSource.includes('<t-radio'), false)
  assert.equal(fileImportSource.includes('class="dialog-topbar"'), true)
  assert.equal(fileImportSource.includes('class="dialog-footer"'), true)
  assert.equal(fileImportSource.includes('width="580px"'), true)
  assert.equal(fileImportSource.includes('min-height: 100px'), true)
  assert.equal(fileImportSource.includes('previewImportBlocks('), true)
  assert.equal(fileImportSource.includes('timeout: 120000'), true)
})

test('upload dialog owns only file format and PDF preset configuration', () => {
  assert.equal(fileImportSource.includes('默认难度'), false)
  assert.equal(fileImportSource.includes("strategyPreset.value = format === 'pdf' ? 'pdf' : 'general'"), true)
  assert.equal(fileImportSource.includes('v-if="importFormat === \'pdf\'"'), true)
  assert.equal(fileImportSource.includes('<t-option value="general"'), true)
  assert.equal(fileImportSource.includes('<t-option value="pdf"'), true)
  assert.equal(fileImportSource.includes('import_mode: props.importMode'), true)
  assert.equal(fileImportSource.includes("default_difficulty: 'medium'"), true)
  assert.equal(fileImportSource.includes("emit('parsed'"), true)
  assert.equal(fileImportSource.includes('useRouter'), false)
  assert.equal(fileImportSource.includes('saveDraft'), false)
})

test('normalizes import file preview response arrays', () => {
  assert.equal(questionApiSource.includes('normalizeImportFilePreviewResponse'), true)
  assert.equal(questionApiSource.includes('Array.isArray(source.items)'), true)
  assert.equal(questionApiSource.includes('Array.isArray(source.errors)'), true)
  assert.equal(questionApiSource.includes('Array.isArray(source.warnings)'), true)
})

test('empty state no longer has action slot', () => {
  assert.equal(source.includes('<template #action>'), false)
  assert.equal(source.includes('emptyImportMenuVisible'), false)
})

test('question review changes trigger debounced draft saves', () => {
  assert.equal(workbenchSource.includes('@changed="saveDebounced"'), true)
  assert.equal(questionReviewSource.includes("changed: []; imported: []"), true)
  assert.equal((questionReviewSource.match(/emit\('changed'\)/g) || []).length, 3)
})

test('workbench is a controlled 90vw modal and no longer uses a route', () => {
  assert.equal(workbenchSource.includes('visible: boolean'), true)
  assert.equal(workbenchSource.includes('kbId: string'), true)
  assert.equal(workbenchSource.includes('setId: string'), true)
  assert.equal(workbenchSource.includes("'update:visible': [value: boolean]"), true)
  assert.equal(workbenchSource.includes('imported: []'), true)
  assert.equal(workbenchSource.includes('abandoned: []'), true)
  assert.equal(workbenchSource.includes('width="90vw"'), true)
  assert.equal(workbenchSource.includes('height: 90vh'), true)
  assert.equal(workbenchSource.includes('useRoute'), false)
  assert.equal(workbenchSource.includes('useRouter'), false)
  assert.equal(routerSource.includes('questionImportWorkbench'), false)
  assert.equal(routerSource.includes('question-import-workbench'), false)
})

test('workbench header owns parse configuration and anomaly summary', () => {
  assert.equal(workbenchSource.includes('v-model="store.defaultDifficulty"'), true)
  assert.equal(workbenchSource.includes('格式'), true)
  assert.equal(workbenchSource.includes('store.strategyPreset'), true)
  assert.equal(workbenchSource.includes('store.summary.total_blocks'), true)
  assert.equal(workbenchSource.includes('anomalyCounts.error'), true)
  assert.equal(workbenchSource.includes('anomalyCounts.warning'), true)
})

test('successful import closes the modal and partial failures stay editable', () => {
  assert.equal(questionReviewSource.includes("emit('imported')"), true)
  assert.equal(questionReviewSource.includes('useRouter'), false)
  assert.equal(questionReviewSource.includes('store.questionErrors = errors.map'), true)
  assert.equal(workbenchSource.includes('@imported="handleImported"'), true)
  assert.equal(workbenchSource.includes("emit('update:visible', false)"), true)
  assert.equal(source.includes('@imported="handleWorkbenchImported"'), true)
  assert.equal(source.includes('await refreshAfterMutation()'), true)
})

test('draft restore is handled from the question set without route navigation', () => {
  assert.equal(source.includes('restoreImportDraft'), true)
  assert.equal(source.includes('applyDraftToWorkbench'), true)
  assert.equal(source.includes('workbenchVisible.value = true'), true)
  assert.equal(source.includes('@parsed="handleFileParsed"'), true)
  assert.equal(source.includes('await saveDraft({'), true)
  assert.equal(source.includes('router.push'), false)
})

test('import modal transitions close the previous layer before opening the next one', () => {
  const openSingleImport = sourceSection(source, 'async function openSingleImport', 'async function openFileImportDialog')
  const restoreImportDraft = sourceSection(source, 'function restoreImportDraft', 'async function startFreshImport')
  const startFreshImport = sourceSection(source, 'async function startFreshImport', 'async function handleFileParsed')
  const handleFileParsed = sourceSection(source, 'async function handleFileParsed', 'async function handleWorkbenchImported')

  assert.equal(source.includes('function closeImportModals()'), true)
  assert.equal(openSingleImport.includes('closeImportModals()'), true)
  assert.equal(restoreImportDraft.includes('fileImportVisible.value = false'), true)
  assert.equal(restoreImportDraft.includes('restoreDraftVisible.value = false'), true)
  assert.equal(restoreImportDraft.includes('headerImportMenuVisible.value = false'), true)
  assert.equal(restoreImportDraft.includes('await nextTick()'), true)
  assert.equal(restoreImportDraft.includes('workbenchVisible.value = true'), true)
  assert.equal(startFreshImport.includes('closeImportModals()'), true)
  assert.equal(startFreshImport.includes('await nextTick()'), true)
  assert.equal(handleFileParsed.includes('fileImportVisible.value = false'), true)
  assert.equal(handleFileParsed.includes('restoreDraftVisible.value = false'), true)
  assert.equal(handleFileParsed.includes('await nextTick()'), true)
  assert.equal(handleFileParsed.includes('workbenchVisible.value = true'), true)
})

test('parsed event ownership stays in the parent and workbench close updates do not trigger abandon', () => {
  const startParsing = sourceSection(fileImportSource, 'async function handleStartParsing', '</script>')
  const parsedEventIndex = startParsing.indexOf("emit('parsed'")
  assert.equal(parsedEventIndex >= 0, true)
  assert.equal(startParsing.slice(parsedEventIndex).includes('closeAndReset()'), false)
  assert.equal(workbenchSource.includes('@update:visible="handleVisibleUpdate"'), false)
  assert.equal(workbenchSource.includes('function handleVisibleUpdate'), false)
})

test('modal layers have an explicit upload, restore, workbench, abandon order', () => {
  assert.equal(fileImportSource.includes(':z-index="2500"'), true)
  assert.equal(source.includes('attach="body"'), true)
  assert.equal(source.includes(':z-index="3200"'), true)
  assert.equal(workbenchSource.includes(':z-index="3500"'), true)
  assert.equal(workbenchSource.includes(':z-index="4500"'), true)
})

test('restoring original block text synchronizes the textarea model', () => {
  assert.equal(blockReviewSource.includes("emit('restore-original'"), true)
  assert.equal(blockReviewSource.includes('store.selectedBlock?.current_text'), true)
  assert.equal(blockReviewSource.includes("store.selectedBlock?.current_text"), true)
})

test('block review uses list, editor, and metadata columns', () => {
  // col-list is now rendered by VirtualBlockList (child component)
  assert.equal(blockReviewSource.includes('<VirtualBlockList'), true)
  assert.equal(blockReviewSource.includes('class="col-editor"'), true)
  assert.equal(blockReviewSource.includes('class="col-meta"'), true)
  assert.equal(blockReviewSource.includes('异常信息'), true)
})

test('question table has row selection and batch actions', () => {
  assert.equal(source.includes("type: 'multiple'"), true)
  assert.equal(source.includes('selectedRowKeys'), true)
  assert.equal(source.includes('onSelectChange'), true)
  assert.equal(source.includes('batchReview'), true)
  assert.equal(source.includes('batchDelete'), true)
  assert.equal(source.includes('批量审核'), true)
  assert.equal(source.includes('批量删除'), true)
  assert.equal(source.includes('清空选择'), true)
})

test('draft status is clickable for single question review', () => {
  assert.equal(source.includes('reviewSingleQuestion'), true)
  assert.equal(source.includes('updateQuestionStatus'), true)
  assert.equal(source.includes("row.status === 'draft'"), true)
  // No old review button in operation column
  assert.equal(source.includes('reviewQuestion(row)'), false)
})

test('reviewed status shows reviewer tooltip', () => {
  assert.equal(source.includes('reviewed_by'), true)
  assert.equal(source.includes('reviewed_at'), true)
  assert.equal(source.includes('t-tooltip'), true)
})

test('question table has pagination', () => {
  assert.equal(source.includes('currentPage'), true)
  assert.equal(source.includes('pageSize'), true)
  assert.equal(source.includes('questionTotal'), true)
  assert.equal(source.includes('onPageChange'), true)
  assert.equal(source.includes('reloadFromFirstPage'), true)
  assert.equal(source.includes('@page-change="onPageChange"'), true)
  assert.equal(source.includes('listQuestions(..., 1, 200)'), false)
})

test('does not use native browser dialogs in question bank components', () => {
  for (const component of [source, bankSource, importSource, fileImportSource]) {
    assert.equal(/window\.(alert|prompt|confirm)\s*\(/.test(component), false)
  }
})

test('defines every questionBank locale key used by question components', () => {
  const componentNames = [
    'QuestionBank.vue',
    'QuestionEditDialog.vue',
    'QuestionGenerateDialog.vue',
    'QuestionImportDialog.vue',
    'QuestionFileImportDialog.vue',
    'QuestionSetDetail.vue',
  ]
  const usedKeys = new Set<string>()
  for (const componentName of componentNames) {
    const component = readFileSync(new URL(`./components/${componentName}`, import.meta.url), 'utf8')
    for (const match of component.matchAll(/questionBank\.([A-Za-z0-9_]+)/g)) {
      usedKeys.add(match[1])
    }
  }

  const translations = (zhCN as any).questionBank || {}
  const missingKeys = [...usedKeys].filter(key => !(key in translations))
  assert.deepEqual(missingKeys, [])
})


// ===== Null-safety regression: anomalies / tags / metadata =====

test('BlockReviewPanel does not access .length or .some() without null guard', () => {
  const rawAnomaliesLen = blockReviewSource.match(/block\.anomalies\.length(?!\s*\?)/g) || []
  assert.equal(rawAnomaliesLen.length, 0, 'block.anomalies.length should not be accessed without guard')
})

test('BlockReviewPanel tags access is guarded', () => {
  const rawTagsLen = blockReviewSource.match(/selectedBlock\.tags\.length(?!\s*\?)/g) || []
  assert.equal(rawTagsLen.length, 0, 'selectedBlock.tags.length should not be accessed without guard')
})

test('QuestionImportWorkbench anomalyCounts guarded against null anomalies', () => {
  // anomalyCounts now delegates to store.getMergedAnomalies which handles Array.isArray guards
  const hasGuard = workbenchSource.includes('store.getMergedAnomalies')
  assert.equal(hasGuard, true, 'anomalyCounts must delegate to getMergedAnomalies for safe anomaly access')
})

test('QuestionFileImportDialog guards result.blocks with Array.isArray', () => {
  assert.equal(fileImportSource.includes('Array.isArray(result.blocks)'), true)
})

test('normalizeImportBlock returns safe defaults for null fields', () => {
  const storeSource = readFileSync(new URL('../../stores/importWorkbench.ts', import.meta.url), 'utf8')
  assert.equal(storeSource.includes('export function normalizeImportBlock'), true)
  assert.equal(storeSource.includes('export function normalizeImportBlocks'), true)
  assert.equal(storeSource.includes('Array.isArray(block.anomalies) ? block.anomalies : []'), true)
  assert.equal(storeSource.includes('tags: normalizeTags(block.tags)'), true)
})

test('setBlocksFromResponse uses normalizeImportBlocks', () => {
  const storeSource = readFileSync(new URL('../../stores/importWorkbench.ts', import.meta.url), 'utf8')
  assert.equal(storeSource.includes('normalizeImportBlocks(input)'), true)
})

test('validateBlocks filters anomalies safely with Array.isArray', () => {
  const storeSource = readFileSync(new URL('../../stores/importWorkbench.ts', import.meta.url), 'utf8')
  assert.equal(storeSource.includes('Array.isArray(block.anomalies) ? block.anomalies : []'), true)
})

// ===== Regression: currentStep must be declared and exported =====

test('currentStep ref is declared in store setup', () => {
  const storeSource = readFileSync(new URL('../../stores/importWorkbench.ts', import.meta.url), 'utf8')
  assert.equal(storeSource.includes("currentStep = ref<WorkbenchStep>('block-review')"), true, 'currentStep ref must be declared')
})

test('currentStep is exported from store return', () => {
  const storeSource = readFileSync(new URL('../../stores/importWorkbench.ts', import.meta.url), 'utf8')
  assert.equal(storeSource.includes('    currentStep,'), true, 'currentStep must be in store return')
})

test('reset() assigns currentStep.value = block-review', () => {
  const storeSource = readFileSync(new URL('../../stores/importWorkbench.ts', import.meta.url), 'utf8')
  assert.equal(storeSource.includes("currentStep.value = 'block-review'"), true, 'reset must set currentStep back to block-review')
})

test('reset() clears blockOrder and blockMap', () => {
  const storeSource = readFileSync(new URL('../../stores/importWorkbench.ts', import.meta.url), 'utf8')
  assert.equal(storeSource.includes('blockOrder.value = []'), true, 'reset must clear blockOrder')
  assert.equal(storeSource.includes('blockMap.value = {}'), true, 'reset must clear blockMap')
})

test('handleFileParsed wraps store init in try/catch', () => {
  const hasTryCatch = source.includes('"打开导入工作台失败"') || source.includes("'打开导入工作台失败'")
  assert.equal(hasTryCatch, true, 'handleFileParsed must catch errors when opening workbench')
})

test('handleFileParsed calls reset then setBlocksFromResponse', () => {
  const handleFn = sourceSection(source, 'async function handleFileParsed', 'async function handleWorkbenchImported')
  const resetIdx = handleFn.indexOf('workbenchStore.reset()')
  const setBlocksIdx = handleFn.indexOf('workbenchStore.setBlocksFromResponse')
  assert.equal(resetIdx >= 0, true, 'handleFileParsed must call reset')
  assert.equal(setBlocksIdx >= 0, true, 'handleFileParsed must call setBlocksFromResponse')
  assert.equal(setBlocksIdx > resetIdx, true, 'setBlocksFromResponse must be called after reset')
})

// ===== P0: JSONL parsing tests =====

test('jsonQuestionImport handles JSON array', () => {
  const adapterSource = readFileSync(new URL('../../utils/jsonQuestionImport.ts', import.meta.url), 'utf8')
  assert.equal(adapterSource.includes("startsWith('[')"), true, 'must handle JSON array')
  assert.equal(adapterSource.includes('Array.isArray(parsed)'), true, 'must validate array')
})

test('jsonQuestionImport handles { questions: [...] } wrapper', () => {
  const adapterSource = readFileSync(new URL('../../utils/jsonQuestionImport.ts', import.meta.url), 'utf8')
  assert.equal(adapterSource.includes('parsed.questions'), true, 'must handle questions wrapper')
  assert.equal(adapterSource.includes('parsed.items'), true, 'must handle items wrapper')
})

test('jsonQuestionImport handles single JSON object', () => {
  const adapterSource = readFileSync(new URL('../../utils/jsonQuestionImport.ts', import.meta.url), 'utf8')
  assert.equal(adapterSource.includes('Single question object'), true, 'must handle single question object')
})

test('jsonQuestionImport handles JSONL with .jsonl detection', () => {
  const adapterSource = readFileSync(new URL('../../utils/jsonQuestionImport.ts', import.meta.url), 'utf8')
  assert.equal(adapterSource.includes("endsWith('.jsonl')"), true, 'must detect .jsonl extension')
  assert.equal(adapterSource.includes('function parseJsonL'), true, 'must have JSONL parser')
})

test('jsonQuestionImport does not silently drop malformed JSONL lines', () => {
  const adapterSource = readFileSync(new URL('../../utils/jsonQuestionImport.ts', import.meta.url), 'utf8')
  assert.equal(adapterSource.includes('JSONL_PARSE_ERROR'), true, 'must emit error for unparseable lines')
  assert.equal(adapterSource.includes('_jsonl_parse_error'), true, 'must track parse errors')
})

// ── Processing button state & drawer ──

test('processing helper functions are imported in QuestionSetDetail', () => {
  assert.equal(source.includes('resolveProcessingStages'), true, 'must import resolveProcessingStages')
  assert.equal(source.includes('resolveProcessingButtonState'), true, 'must import resolveProcessingButtonState')
  assert.equal(source.includes('PROCESSING_STAGE_STATUS_LABELS'), true, 'must import PROCESSING_STAGE_STATUS_LABELS')
  assert.equal(source.includes('PROCESSING_BUTTON_LABELS'), true, 'must import PROCESSING_BUTTON_LABELS')
})

test('processing button is conditionally rendered based on state', () => {
  // Button must check state before rendering
  assert.equal(source.includes("processingButton.state !== 'hidden'"), true, 'must hide button when state is hidden')
  // Running and paused states both show t-loading spinner in icon slot
  assert.equal(source.includes("processingButton.state === 'running' || processingButton.state === 'paused'"), true, 'must use t-loading for both running and paused')
  // Click opens drawer
  assert.equal(source.includes('processingDrawerVisible = true'), true, 'must open drawer on click')
  // Button is round shape with tooltip
  assert.equal(source.includes('shape="round"'), true, 'must use round shape for pill button style')
  assert.equal(source.includes('processingButtonTooltip'), true, 'must use tooltip for status text')
  // Button must NOT use t-button loading prop (that would disable click)
  assert.equal(source.includes(':loading="processingButton.state ==='), false, 'must not use t-button loading prop')
})

test('processing drawer renders waterfall timeline matching knowledge timeline structure', () => {
  assert.equal(source.includes('processingDrawerVisible'), true, 'must have drawer visibility state')
  assert.equal(source.includes('<t-drawer'), true, 'must have t-drawer component')
  // Secondary drawer class (wider, no header prop)
  assert.equal(source.includes('kp-secondary-drawer'), true, 'must use kp-secondary-drawer class')
  // Waterfall shell/head/body structure
  assert.equal(source.includes('kp-shell'), true, 'must use kp-shell')
  assert.equal(source.includes('kp-head'), true, 'must use kp-head')
  assert.equal(source.includes('kp-head-toolbar'), true, 'must use kp-head-toolbar')
  assert.equal(source.includes('kp-head-doc-title'), true, 'must use kp-head-doc-title')
  assert.equal(source.includes('kp-head-status-tag'), true, 'must use kp-head-status-tag')
  assert.equal(source.includes('kp-head-meta'), true, 'must use kp-head-meta')
  assert.equal(source.includes('处理流水线'), true, 'meta must include 处理流水线')
  // Body and ruler
  assert.equal(source.includes('kp-body'), true, 'must use kp-body')
  assert.equal(source.includes('kp-ruler'), true, 'must use kp-ruler')
  assert.equal(source.includes('kp-ruler-track'), true, 'must use kp-ruler-track')
  assert.equal(source.includes('kp-scroll'), true, 'must use kp-scroll')
  // Rows and cells
  assert.equal(source.includes('kp-rows'), true, 'must use kp-rows')
  assert.equal(source.includes('kp-cell-name'), true, 'must use kp-cell-name')
  assert.equal(source.includes('kp-cell-dur'), true, 'must use kp-cell-dur')
  assert.equal(source.includes('kp-cell-bar'), true, 'must use kp-cell-bar')
  assert.equal(source.includes('kp-bar'), true, 'must use kp-bar')
  assert.equal(source.includes('kp-status-dot'), true, 'must use kp-status-dot')
  assert.equal(source.includes('kp-name-text'), true, 'must use kp-name-text')
  assert.equal(source.includes('kp-name-kind'), true, 'must use kp-name-kind')
  // Old stage list classes removed
  assert.equal(source.includes('kp-stage-list'), false, 'old kp-stage-list must be removed')
  assert.equal(source.includes('kp-stage-row'), false, 'old kp-stage-row must be removed')
  assert.equal(source.includes('processing-drawer-hint'), false, 'old processing-drawer-hint must be removed')
  // Detail panel for row click
  assert.equal(source.includes('kp-detail'), true, 'must use kp-detail panel')
  assert.equal(source.includes('kp-detail-head'), true, 'must use kp-detail-head')
  assert.equal(source.includes('kp-tabs'), true, 'must use kp-tabs')
  assert.equal(source.includes('概览'), true, 'must have 概览 tab')
  assert.equal(source.includes('输入'), true, 'must have 输入 tab')
  assert.equal(source.includes('输出'), true, 'must have 输出 tab')
  assert.equal(source.includes('原始 JSON'), true, 'must have 原始 JSON tab')
  assert.equal(source.includes('kp-body-with-detail'), true, 'must use kp-body-with-detail')
  // kp-name-reason removed — reason only in detail panel
  assert.equal(source.includes('kp-name-reason'), false, 'kp-name-reason must be removed')
  // ROOT row shows aggregate status, not "ROOT"
  assert.equal(source.includes('qpRowKindLabel'), true, 'must use qpRowKindLabel')
  assert.equal(source.includes('qpRootStatusLabel'), true, 'must use qpRootStatusLabel')
  assert.equal(source.includes("return '部分暂停'"), true, 'qpRootStatusLabel must include 部分暂停')
  assert.equal(source.includes("return '进行中'"), true, 'qpRootStatusLabel must include 进行中')
  assert.equal(source.includes("return '待人工审核'"), true, 'qpRootStatusLabel must include 待人工审核')
  // Paused stage duration shows "暂停" instead of "—"
  assert.equal(source.includes('qp-paused-duration'), true, 'must use qp-paused-duration for paused dur')
  assert.equal(source.includes("row.status === 'paused'"), true, 'must check paused for duration display')
  // No fake times
  assert.equal(source.includes('const offset = i * 500'), false, 'must not use fake offset times')
  assert.equal(source.includes('dur = 500'), false, 'must not use fake duration')
  // Paused loading is orange
  assert.equal(source.includes('qp-loading-warning'), true, 'must have qp-loading-warning class')
  // Reason shown in detail panel, not on waterfall row
  assert.equal(source.includes('selectedProcessingRow.reason'), true, 'reason must be in detail panel')
  // Paused icon is NOT pause-circle
  assert.equal(source.includes('pause-circle-filled'), false, 'must not use pause-circle-filled')
  assert.equal(source.includes('pause-circle'), false, 'must not use pause-circle')
})

test('processing drawer header shows status tag for paused and ready_for_review', () => {
  // Header status tag computed values
  assert.equal(source.includes("'部分暂停'"), true, 'header status must include 部分暂停 for paused')
  assert.equal(source.includes("'待人工审核'"), true, 'ready_for_review header status must show 待人工审核')
  // Old banner hint removed (waterfall shows reason per row instead)
  assert.equal(source.includes('部分阶段因配置缺失已暂停'), false, 'old alert banner text must be removed')
})

test('processing resolve functions implement correct priority and 4-stage pipeline', () => {
  const apiSource = readFileSync(new URL('../../api/question.ts', import.meta.url), 'utf8')

  // resolveProcessingStages handles paused stages from config
  assert.equal(apiSource.includes("status === 'paused'"), true, 'must define paused status')
  assert.equal(apiSource.includes("status === 'failed'"), true, 'must define failed status')
  assert.equal(apiSource.includes("status === 'running'"), true, 'must define running status')

  // resolveProcessingButtonState implements priority order
  const buttonFn = sourceSection(apiSource, 'export function resolveProcessingButtonState', 'export function')
  assert.equal(buttonFn.includes("stage === 'failed'"), true, 'failed check must have highest priority')
  assert.equal(buttonFn.includes("status === 'running'"), true, 'running check must precede paused')
  assert.equal(buttonFn.includes("status === 'paused'"), true, 'paused check must precede ready_for_review')

  // STAGE_ORDER has 4 items, no ready_for_review
  assert.equal(apiSource.includes("STAGE_ORDER = ['draft_imported', 'indexing', 'auto_tagging', 'syllabus_checking']"), true, 'STAGE_ORDER must have 4 stages without ready_for_review')
  assert.equal(apiSource.includes("ready_for_review']"), false, 'STAGE_ORDER must not include ready_for_review')

  // Stages derivation checks both enabled flags
  assert.equal(apiSource.includes('auto_tagging_enabled'), true, 'must check auto_tagging_enabled')
  assert.equal(apiSource.includes('syllabus_check_enabled'), true, 'must check syllabus_check_enabled')
  assert.equal(apiSource.includes('skipped_auto_tagging_reason'), true, 'must use skipped_auto_tagging_reason')
  assert.equal(apiSource.includes('skipped_syllabus_reason'), true, 'must use skipped_syllabus_reason')
})

test('processing button uses dynamic theme and icon per state', () => {
  // Theme mapping includes all states
  assert.equal(source.includes("failed: 'danger'"), true, 'failed state must use danger theme')
  assert.equal(source.includes("paused: 'warning'"), true, 'paused state must use warning theme')
  assert.equal(source.includes("running: 'primary'"), true, 'running state must use primary theme')
  assert.equal(source.includes("ready_for_review: 'success'"), true, 'ready_for_review must use success theme')

  // Icon mapping no longer uses pause-circle for paused
  assert.equal(source.includes("pause-circle"), false, 'must not use pause-circle icon anywhere')
  assert.equal(source.includes("failed: 'close-circle'"), true, 'failed must use close-circle icon')
  // Paused and running both use t-loading via template condition
  assert.equal(source.includes("processingButton.state === 'running' || processingButton.state === 'paused'"), true, 'paused must also show t-loading')
})

test('processing button tooltip shows progress count when running', () => {
  // Running state shows X/Y progress in tooltip
  assert.equal(source.includes('btn.completedCount'), true, 'must show completedCount')
  assert.equal(source.includes('btn.totalCount'), true, 'must show totalCount')
  assert.equal(source.includes("btn.state === 'running'"), true, 'must conditionally show progress count')
  // Tooltip content computed
  assert.equal(source.includes('processingButtonTooltip'), true, 'must have tooltip property')
  assert.equal(source.includes('题目处理中'), true, 'running tooltip must mention processing')
})

test('polling stops at terminal stages but button remains visible', () => {
  // Polling still stops at ready_for_review / failed / ''
  assert.equal(source.includes("stage === 'ready_for_review' || stage === 'failed' || stage === ''"), true, 'must stop polling at terminal stages')
  // But the old banner is removed (replaced by button + drawer)
  assert.equal(source.includes('processing-banner'), false, 'old processing-banner class must be removed')
  assert.equal(source.includes('stageLabel(processingStatus.stage)'), false, 'old stageLabel call must be removed')
})

test('removed standalone add-question, generate, export controls; drawer is waterfall', () => {
  // No standalone "新增题目" button
  assert.equal(source.includes('新增题目'), false, 'standalone add-question button must be removed')
  // No generate dialog
  assert.equal(source.includes('QuestionGenerateDialog'), false, 'QuestionGenerateDialog must not be referenced')
  // No export dialog
  assert.equal(source.includes('exportToEvaluationDataset'), false, 'exportToEvaluationDataset must not be referenced')
  assert.equal(source.includes('导出评测集'), false, 'export evaluation dataset must not be referenced')
  // Import menu is the only primary action
  assert.equal(source.includes('openManualImport'), true, 'manual import must be in the import menu')
  // Drawer is waterfall, not old card list
  assert.equal(source.includes('kp-shell'), true, 'drawer must use waterfall kp-shell')
  assert.equal(source.includes('kp-stage-list'), false, 'old kp-stage-list must be removed')
  assert.equal(source.includes('processing-stage-item'), false, 'old processing-stage-item must be removed')
  assert.equal(source.includes('pause-circle'), false, 'pause-circle icon must not be used')
  assert.equal(source.includes('processing-drawer-hint'), false, 'old hint alert must be removed')
  // Paused label remains "部分暂停"
  assert.equal(questionApiSource.includes("paused: '部分暂停'"), true, 'paused button label must remain 部分暂停')
})
