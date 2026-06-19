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

test('renders the question set sidebar as a four-column list with a popup menu', () => {
  assert.equal(bankSource.includes('class="set-list-header"'), true)
  assert.equal(bankSource.includes('v-for="(set, index) in filteredQuestionSets"'), true)
  assert.equal(bankSource.includes('{{ index + 1 }}'), true)
  assert.equal(bankSource.includes('{{ set.question_count || 0 }} 题'), true)
  assert.equal(bankSource.includes('<t-popup'), true)
  assert.equal(bankSource.includes('openRenameDialog(set)'), true)
  assert.equal(bankSource.includes('confirmDeleteSet(set)'), true)
  assert.equal(bankSource.includes('source_type'), false)
  assert.equal(bankSource.includes('class="set-meta"'), false)
})

test('top-level import menu offers single and disabled batch import only', () => {
  assert.equal(source.includes('openSingleImport'), true)
  assert.equal(source.includes('单个导入'), true)
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
