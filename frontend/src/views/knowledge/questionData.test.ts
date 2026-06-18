import assert from 'node:assert/strict'
import test from 'node:test'

import {
  classifyQuestionImportItems,
  normalizeQuestionText,
  parseQuestionImportInput,
  questionFingerprint,
  resolveQuestionRows,
  resolveQuestionTotal,
  selectQuestionImportItems,
} from './questionData.ts'

test('converts evaluation JSONL into a question import item', () => {
  const input = '{"question":"在 NeoForge 中，为什么注册 Block、Item、EntityType 等对象时推荐使用 DeferredRegister？","reference_answer":"因为 DeferredRegister 会把注册动作延迟到正确的注册阶段执行，避免类加载顺序错误、注册表冻结后注册、对象未被游戏识别等问题。","reference_contexts":[]}'
  const result = parseQuestionImportInput(input)

  assert.deepEqual(result.errors, [])
  assert.deepEqual(result.warnings, [])
  assert.equal(result.items.length, 1)
  assert.equal(result.items[0].stem_text, '在 NeoForge 中，为什么注册 Block、Item、EntityType 等对象时推荐使用 DeferredRegister？')
  assert.equal(result.items[0].answer_text, '因为 DeferredRegister 会把注册动作延迟到正确的注册阶段执行，避免类加载顺序错误、注册表冻结后注册、对象未被游戏识别等问题。')
  assert.equal(result.items[0].question_type, 'short_answer')
  assert.equal(result.items[0].difficulty, 'medium')
})

test('preserves evaluation contexts and maps chunk references', () => {
  const input = JSON.stringify({
    question: '题干',
    reference_answer: '答案',
    reference_contexts: [
      { text: '上下文', knowledge_id: 'knowledge-1', chunk_id: 'chunk-1' },
      { text: '补充上下文' },
    ],
  })
  const result = parseQuestionImportInput(input)

  assert.deepEqual(result.items[0].evidence_chunk_ids, ['chunk-1'])
  assert.equal(result.items[0].source_knowledge_id, 'knowledge-1')
  assert.deepEqual(result.items[0].answer_body.reference_contexts, [
    { text: '上下文', knowledge_id: 'knowledge-1', chunk_id: 'chunk-1' },
    { text: '补充上下文' },
  ])
})

test('reports an actionable error when both stem fields are missing', () => {
  const result = parseQuestionImportInput('{"reference_answer":"答案"}')

  assert.equal(result.items.length, 0)
  assert.deepEqual(result.errors, [{
    line_number: 1,
    message: '缺少题干字段。请提供 stem_text，或使用评测集格式中的 question 字段。',
  }])
})

test('allows missing answers and returns a draft warning', () => {
  const result = parseQuestionImportInput('{"question":"只有题干"}')

  assert.equal(result.items.length, 1)
  assert.equal(result.items[0].answer_text, '')
  assert.deepEqual(result.warnings, [{
    line_number: 1,
    message: '该题缺少答案，将以草稿导入；审核或导出前需要补全。',
  }])
})

test('resolves question rows from supported response shapes', () => {
  const rows = [{ id: 'question-1' }]

  assert.deepEqual(resolveQuestionRows({ data: rows }), rows)
  assert.deepEqual(resolveQuestionRows({ data: { data: rows } }), rows)
  assert.deepEqual(resolveQuestionRows({ items: rows }), rows)
  assert.deepEqual(resolveQuestionRows({ list: rows }), rows)
  assert.deepEqual(resolveQuestionRows(rows), rows)
  assert.deepEqual(resolveQuestionRows({ data: null }), [])
})

test('resolves the total independently from the visible rows', () => {
  const rows = [{ id: 'question-1' }]

  assert.equal(resolveQuestionTotal({ total: 3, data: rows }, rows), 3)
  assert.equal(resolveQuestionTotal({ data: { total: 4, data: rows } }, rows), 4)
  assert.equal(resolveQuestionTotal(rows, rows), 1)
})

test('normalizes whitespace when building duplicate fingerprints', () => {
  assert.equal(normalizeQuestionText('  题干\n  内容  '), '题干 内容')
  assert.equal(
    questionFingerprint({ question_type: 'short_answer', stem_text: ' 题干 ', answer_text: '答  案' }),
    'short_answer|题干|答 案',
  )
})

test('classifies duplicates within the import and against existing questions', () => {
  const parsed = parseQuestionImportInput([
    { question: '已有题', reference_answer: '已有答案' },
    { question: '新题', reference_answer: '新答案' },
    { question: ' 新题 ', reference_answer: '新答案' },
  ].map(item => JSON.stringify(item)).join('\n'))
  const existing = [{
    question_type: 'short_answer',
    stem_text: '已有题',
    answer_text: '已有答案',
  }]

  const classified = classifyQuestionImportItems(parsed.items, existing)

  assert.equal(classified.uniqueItems.length, 1)
  assert.equal(classified.uniqueItems[0].stem_text, '新题')
  assert.deepEqual(classified.duplicateItems.map(item => item.line_number), [1, 3])
  assert.deepEqual(selectQuestionImportItems(parsed.items, classified, false), classified.uniqueItems)
  assert.deepEqual(selectQuestionImportItems(parsed.items, classified, true), parsed.items)
})

test('returns no importable items when every parsed question already exists', () => {
  const parsed = parseQuestionImportInput('{"question":"已有题","reference_answer":"已有答案"}')
  const classified = classifyQuestionImportItems(parsed.items, [{
    question_type: 'short_answer',
    stem_text: '已有题',
    answer_text: '已有答案',
  }])

  assert.equal(classified.uniqueItems.length, 0)
  assert.equal(classified.duplicateItems.length, 1)
  assert.deepEqual(selectQuestionImportItems(parsed.items, classified, false), [])
})
