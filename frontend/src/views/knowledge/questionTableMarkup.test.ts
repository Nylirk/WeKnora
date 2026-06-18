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
  assert.equal(source.includes('questionTotal'), false)
})

test('renders only spaced edit and delete row actions', () => {
  const operationSlot = source.match(/<template #operation="\{ row \}">([\s\S]*?)<\/template>/)?.[1] || ''

  assert.equal(operationSlot.includes('<t-space size="small">'), true)
  assert.equal(operationSlot.includes('openEditDialog(row)'), true)
  assert.equal(operationSlot.includes('removeQuestion(row)'), true)
  assert.equal(operationSlot.includes('reviewQuestion(row)'), false)
  assert.equal(source.includes('updateQuestionStatus'), true)
})

test('passes current questions into duplicate detection and syncs totals to the selected set', () => {
  assert.equal(source.includes(':current-questions="questions"'), true)
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

test('offers JSON, Word, and PDF import entry points', () => {
  assert.equal(source.includes('openJsonImport'), true)
  assert.equal(source.includes("openFileImport('word')"), true)
  assert.equal(source.includes("openFileImport('pdf')"), true)
  assert.equal(source.includes("questionBank.jsonImport"), true)
  assert.equal(source.includes("questionBank.wordImport"), true)
  assert.equal(source.includes("questionBank.pdfImport"), true)
  assert.equal(source.includes('QuestionFileImportDialog'), true)
  assert.equal(source.includes(':import-type="fileImportType"'), true)
  assert.equal(source.includes(':key="`${fileImportType}-${fileImportSession}`"'), true)
  assert.equal((source.match(/class="import-type-item" disabled/g) || []).length, 0)
})

test('closes import type menu before opening import dialogs', () => {
  assert.equal(source.includes('headerImportMenuVisible'), true)
  assert.equal(source.includes('closeAllImportMenus'), true)
  assert.equal(source.includes('await closeAllImportMenus()'), true)
  assert.equal(source.includes('v-model:visible="headerImportMenuVisible"'), true)
  // openFileImport must close menu, destroy old dialog, then open fresh session
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

test('file import dialog disables overlay close and resets cancellable preview sessions', () => {
  assert.equal(fileImportSource.includes(':close-on-overlay-click="false"'), true)
  assert.equal(fileImportSource.includes('new AbortController()'), true)
  assert.equal(fileImportSource.includes('abortCurrentRequest'), true)
  assert.equal(fileImportSource.includes('cleanupDialogState'), true)
  assert.equal(fileImportSource.includes('closeAndReset'), true)
  assert.equal(fileImportSource.includes('activePreviewRequestId'), true)
  assert.equal(fileImportSource.includes('previewImportFile('), true)
  assert.equal(fileImportSource.includes('signal: controller.signal'), true)
  assert.equal(fileImportSource.includes('timeout: 120000'), true)
})

test('file import preview handles nullable arrays safely', () => {
  assert.equal(fileImportSource.includes('previewWarnings'), true)
  assert.equal(fileImportSource.includes('previewWarnings.length'), true)
  // Must use safe computed, not raw null-unsafe property
  assert.equal(fileImportSource.includes('previewResult.warnings.length'), false)
  assert.equal(fileImportSource.includes('previewStats'), true)
  assert.equal(fileImportSource.includes('rawTextPreview'), true)
  assert.equal(fileImportSource.includes('Array.isArray(previewResult.value?.warnings)'), true)
  assert.equal(fileImportSource.includes('Array.isArray(previewResult.value?.items)'), true)
  assert.equal(fileImportSource.includes('Array.isArray(previewResult.value?.errors)'), true)
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

test('narrows file import dialog width to 560px', () => {
  assert.equal(fileImportSource.includes('width="560px"'), true)
  assert.equal(fileImportSource.includes(':width="680"'), false)
  assert.equal(fileImportSource.includes(':width="800"'), false)
})

test('opens a non-modal drawer with tabs for questions and raw text', () => {
  assert.equal(fileImportSource.includes('t-drawer'), true)
  assert.equal(fileImportSource.includes('previewDrawerVisible'), true)
  assert.equal(fileImportSource.includes('previewDrawerTitle'), true)
  assert.equal(fileImportSource.includes('解析预览'), true)
  assert.equal(fileImportSource.includes('attach="body"'), true)
  assert.equal(fileImportSource.includes(':show-overlay="false"'), true)
  assert.equal(fileImportSource.includes('size="440px"'), true)
  assert.equal(fileImportSource.includes('size="520px"'), false)
  // Drawer uses t-tabs for questions + raw text
  assert.equal(fileImportSource.includes('t-tabs'), true)
  assert.equal(fileImportSource.includes('t-tab-panel'), true)
  assert.equal(fileImportSource.includes('drawerTab'), true)
  assert.equal(fileImportSource.includes('全部'), true)
})

test('raw text preview moved from dialog to drawer', () => {
  // Raw text t-collapse should not be in the dialog body
  assert.equal(fileImportSource.includes('class="raw-text"'), false)
  // Raw text is in the drawer now
  assert.equal(fileImportSource.includes('drawer-raw-text'), true)
})

test('preview drawer opens on successful parse and closes on cleanup', () => {
  assert.equal(fileImportSource.includes('previewDrawerVisible.value = true'), true)
  // cleanupDialogState must close drawer
  const cleanupBody = fileImportSource.match(/function cleanupDialogState\(\) \{([\s\S]*?)  \}/)?.[1] || ''
  assert.equal(cleanupBody.includes('previewDrawerVisible.value = false'), true)
  // onFileSelected must also close drawer
  const onFileBody = fileImportSource.match(/function onFileSelected\([^)]+\) \{([\s\S]*?)  \}/)?.[1] || ''
  assert.equal(onFileBody.includes('previewDrawerVisible.value = false'), true)
})

test('has a view-results button to reopen the drawer', () => {
  assert.equal(fileImportSource.includes('查看解析结果'), true)
  assert.equal(fileImportSource.includes('previewDrawerVisible = true'), true)
})

test('import button stays in dialog footer, drawer has no footer', () => {
  assert.equal(fileImportSource.includes('doConfirmImport'), true)
  assert.equal(fileImportSource.includes(':footer="false"'), true)
})

test('dialog shifted left when drawer open', () => {
  assert.equal(fileImportSource.includes('dialog-shifted-left'), true)
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

test('stats moved from main dialog to drawer', () => {
  assert.equal(fileImportSource.includes('drawer-stats'), true)
  assert.equal(fileImportSource.includes('previewStats.detected_questions'), true)
  assert.equal(fileImportSource.includes('previewStats.with_answer'), true)
  assert.equal(fileImportSource.includes('previewStats.without_answer'), true)
})

test('duplicate detection uses groups with raw text comparison', () => {
  assert.equal(fileImportSource.includes('duplicateCount'), true)
  assert.equal(fileImportSource.includes('duplicateMode'), true)
  assert.equal(fileImportSource.includes('duplicateGroups'), true)
  assert.equal(fileImportSource.includes('getItemRawText'), true)
  assert.equal(fileImportSource.includes('source_payload'), true)
  assert.equal(fileImportSource.includes('dup-group'), true)
  assert.equal(fileImportSource.includes('dup-raw'), true)
  assert.equal(fileImportSource.includes('重复组 #'), true)
  assert.equal(fileImportSource.includes('重复原因'), true)
  assert.equal(fileImportSource.includes('当前仅检测本次文件内重复'), true)
  assert.equal(fileImportSource.includes('保留疑似重复题'), true)
  assert.equal(fileImportSource.includes('忽略疑似重复题'), true)
})

test('staged flow action with duplicateMode defaulting to skip', () => {
  assert.equal(fileImportSource.includes('handleFlowAction'), true)
  assert.equal(fileImportSource.includes('flowActionLabel'), true)
  assert.equal(fileImportSource.includes('flowActionDisabled'), true)
  assert.equal(fileImportSource.includes('!previewResult'), true)
  // duplicateMode defaults to 'skip' — no empty-string blocking state
  assert.equal(fileImportSource.includes("duplicateMode = ref<'include' | 'skip'>('skip')"), true)
  assert.equal(fileImportSource.includes('doPreviewParse'), true)
  assert.equal(fileImportSource.includes('doConfirmImport'), true)
  assert.equal(fileImportSource.includes('itemsToImport'), true)
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
