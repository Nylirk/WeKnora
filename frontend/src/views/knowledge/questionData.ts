import type {
  ImportQuestionError,
  ImportQuestionItem,
  QuestionDifficulty,
  QuestionType,
} from '../../api/question.ts'

export type QuestionImportParseResult = {
  items: ImportQuestionItem[]
  errors: ImportQuestionError[]
  warnings: ImportQuestionError[]
}

type QuestionFingerprintInput = {
  question_type?: unknown
  stem_text?: unknown
  answer_text?: unknown
}

export type QuestionImportClassification = {
  uniqueItems: ImportQuestionItem[]
  duplicateItems: ImportQuestionItem[]
  duplicateGroups: QuestionImportDuplicateGroup[]
}

const asRecord = (value: unknown): Record<string, any> =>
  value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, any> : {}

const asStringArray = (value: unknown): string[] =>
  Array.isArray(value) ? value.filter(item => typeof item === 'string' && item.trim()).map(item => item.trim()) : []

function normalizeImportItem(
  value: unknown,
  lineNumber: number,
  result: QuestionImportParseResult,
): ImportQuestionItem | null {
  const raw = asRecord(value)
  if (!Object.keys(raw).length) {
    result.errors.push({ line_number: lineNumber, message: '题目必须是 JSON 对象。' })
    return null
  }

  const stemText = typeof (raw.stem_text ?? raw.question) === 'string'
    ? String(raw.stem_text ?? raw.question).trim()
    : ''
  if (!stemText) {
    result.errors.push({
      line_number: lineNumber,
      message: '缺少题干字段。请提供 stem_text，或使用评测集格式中的 question 字段。',
    })
    return null
  }

  const answerText = typeof (raw.answer_text ?? raw.reference_answer) === 'string'
    ? String(raw.answer_text ?? raw.reference_answer).trim()
    : ''
  if (!answerText) {
    result.warnings.push({
      line_number: lineNumber,
      message: '该题缺少答案，将以草稿导入；审核或导出前需要补全。',
    })
  }

  const referenceContexts = Array.isArray(raw.reference_contexts) ? raw.reference_contexts : []
  const contextRecords = referenceContexts.map(asRecord).filter(context => Object.keys(context).length)
  const contextChunkIds = contextRecords
    .map(context => typeof context.chunk_id === 'string' ? context.chunk_id.trim() : '')
    .filter(Boolean)
  const contextKnowledgeId = contextRecords
    .map(context => typeof context.knowledge_id === 'string' ? context.knowledge_id.trim() : '')
    .find(Boolean) || ''

  const answerBody = { ...asRecord(raw.answer_body) }
  if (referenceContexts.length) {
    // ImportQuestionItem has no source_payload/extraction_metadata fields yet.
    // Keep evaluation contexts in a persisted JSON field instead of dropping them.
    answerBody.reference_contexts = referenceContexts
  }

  return {
    line_number: lineNumber,
    question_type: (typeof raw.question_type === 'string' ? raw.question_type : 'short_answer') as QuestionType,
    stem_text: stemText,
    question_body: asRecord(raw.question_body),
    answer_text: answerText,
    answer_body: answerBody,
    analysis_text: typeof (raw.analysis_text ?? raw.explanation) === 'string'
      ? String(raw.analysis_text ?? raw.explanation).trim()
      : '',
    grading_rubric: asRecord(raw.grading_rubric),
    difficulty: (typeof raw.difficulty === 'string' ? raw.difficulty : 'medium') as QuestionDifficulty,
    knowledge_points: asStringArray(raw.knowledge_points),
    tags: asStringArray(raw.tags),
    source_knowledge_id: typeof raw.source_knowledge_id === 'string'
      ? raw.source_knowledge_id.trim()
      : contextKnowledgeId,
    evidence_chunk_ids: [...new Set([
      ...asStringArray(raw.evidence_chunk_ids),
      ...contextChunkIds,
    ])],
  }
}

export function parseQuestionImportInput(input: string): QuestionImportParseResult {
  const result: QuestionImportParseResult = { items: [], errors: [], warnings: [] }
  const text = input.trim()
  if (!text) return result

  try {
    const parsed = JSON.parse(text)
    const values = Array.isArray(parsed) ? parsed : [parsed]
    values.forEach((value, index) => {
      const item = normalizeImportItem(value, index + 1, result)
      if (item) result.items.push(item)
    })
    return result
  } catch {
    text.split(/\r?\n/).forEach((line, index) => {
      if (!line.trim()) return
      try {
        const item = normalizeImportItem(JSON.parse(line), index + 1, result)
        if (item) result.items.push(item)
      } catch {
        result.errors.push({ line_number: index + 1, message: 'JSON 解析失败。' })
      }
    })
    return result
  }
}

export function resolveQuestionRows<T>(page: any): T[] {
  if (Array.isArray(page)) return page
  if (Array.isArray(page?.data)) return page.data
  if (Array.isArray(page?.data?.data)) return page.data.data
  if (Array.isArray(page?.items)) return page.items
  if (Array.isArray(page?.list)) return page.list
  return []
}

export function resolveQuestionTotal(page: any, rows: unknown[]): number {
  if (typeof page?.total === 'number') return page.total
  if (typeof page?.data?.total === 'number') return page.data.total
  return rows.length
}

export function normalizeQuestionText(value: unknown): string {
  return String(value || '').trim().replace(/\s+/g, ' ')
}

export function questionFingerprint(question: QuestionFingerprintInput): string {
  return [
    question.question_type || 'short_answer',
    normalizeQuestionText(question.stem_text),
    normalizeQuestionText(question.answer_text),
  ].join('|')
}

export function classifyQuestionImportItems(
  items: ImportQuestionItem[],
  existingQuestions: QuestionFingerprintInput[],
): QuestionImportClassification {
  const existingFingerprints = new Set(existingQuestions.map(questionFingerprint))
  const seenFingerprints = new Set<string>()
  const uniqueItems: ImportQuestionItem[] = []
  const duplicateItems: ImportQuestionItem[] = []

  for (const item of items) {
    const fingerprint = questionFingerprint(item)
    if (existingFingerprints.has(fingerprint) || seenFingerprints.has(fingerprint)) {
      duplicateItems.push(item)
      continue
    }
    seenFingerprints.add(fingerprint)
    uniqueItems.push(item)
  }

  return { uniqueItems, duplicateItems, duplicateGroups: [] }
}

export function classifyQuestionImportItemsWithinFile(
  items: ImportQuestionItem[],
): QuestionImportClassification {
  const seenFingerprints = new Map<string, number>() // fingerprint → index into uniqueItems
  const uniqueItems: ImportQuestionItem[] = []
  const duplicateItems: ImportQuestionItem[] = []
  const groupMap = new Map<string, QuestionImportDuplicateGroup>()

  items.forEach((item, index) => {
    const fp = questionFingerprint(item)
    const existingIdx = seenFingerprints.get(fp)
    if (existingIdx !== undefined) {
      duplicateItems.push(item)
      const group = groupMap.get(fp)
      if (group) {
        group.duplicateItems.push(item)
      }
    } else {
      seenFingerprints.set(fp, index)
      uniqueItems.push(item)
      groupMap.set(fp, {
        fingerprint: fp,
        firstIndex: index + 1, // 1-based for display
        firstItem: item,
        duplicateItems: [],
      })
    }
  })

  return {
    uniqueItems,
    duplicateItems,
    duplicateGroups: [...groupMap.values()].filter(g => g.duplicateItems.length > 0),
  }
}

export function selectQuestionImportItems(
  items: ImportQuestionItem[],
  classified: QuestionImportClassification,
  allowDuplicates: boolean,
): ImportQuestionItem[] {
  return allowDuplicates ? items : classified.uniqueItems
}

export type QuestionImportDuplicateGroup = {
  fingerprint: string
  firstIndex: number
  firstItem: ImportQuestionItem
  duplicateItems: ImportQuestionItem[]
}
