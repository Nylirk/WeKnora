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
  assert.equal(source.includes('updateQuestionStatus'), false)
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
  assert.equal(source.includes('emptyImportMenuVisible'), true)
  assert.equal(source.includes('closeAllImportMenus'), true)
  assert.equal(source.includes('await closeAllImportMenus()'), true)
  assert.equal(source.includes('v-model:visible="headerImportMenuVisible"'), true)
  assert.equal(source.includes('v-model:visible="emptyImportMenuVisible"'), true)
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
