import type { ImportBlock, BlockPreviewSummary } from '@/api/question_block'
import { normalizeTags, normalizeAnomalySeverity } from '@/stores/importWorkbench'

interface JsonQuestionItem {
  question_number?: number
  line_number?: number
  question_type?: string
  type?: string
  stem_text?: string
  stem?: string
  question?: string
  title?: string
  question_body?: unknown
  body?: unknown
  answer_text?: string
  answer?: string
  analysis_text?: string
  analysis?: string
  explanation?: string
  difficulty?: string
  tags?: string[]
  knowledge_points?: string[]
  options?: unknown[]
  status?: string
  [key: string]: unknown
}

function resolveQuestionNumber(item: JsonQuestionItem, index: number): number | null {
  if (typeof item.question_number === 'number' && item.question_number > 0) return item.question_number
  if (typeof item.line_number === 'number' && item.line_number > 0) return item.line_number
  return index + 1
}

function validateJsonQuestionItem(item: JsonQuestionItem): ImportBlock['anomalies'] {
  const anomalies: ImportBlock['anomalies'] = []
  const hasStem = typeof item.stem_text === 'string' ||
    typeof item.stem === 'string' ||
    typeof item.question === 'string' ||
    typeof item.title === 'string' ||
    (item.question_body && typeof item.question_body === 'object') ||
    (item.body && typeof item.body === 'object')
  const hasAnswer = typeof item.answer_text === 'string' ||
    typeof item.answer === 'string'
  const hasType = typeof item.question_type === 'string' ||
    typeof item.type === 'string'

  if (!hasStem) {
    anomalies.push({ code: 'MISSING_STEM', severity: 'error', message: '缺少题干 (stem_text / question / question_body)' })
  }
  if (!hasAnswer) {
    anomalies.push({ code: 'MISSING_ANSWER', severity: 'warning', message: '缺少答案 (answer_text / answer)' })
  }
  if (!hasType) {
    anomalies.push({ code: 'MISSING_QUESTION_TYPE', severity: 'warning', message: '缺少题型 (question_type)' })
  }
  return anomalies
}

function parseJsonText(text: string): JsonQuestionItem[] {
  const trimmed = text.trim()
  if (!trimmed) return []

  // JSON array
  if (trimmed.startsWith('[')) {
    const parsed = JSON.parse(trimmed)
    if (Array.isArray(parsed)) return parsed.map(item => (typeof item === 'object' && item !== null ? item : {}))
    return []
  }

  // JSON object wrapper: { "questions": [...] } or { "items": [...] }
  if (trimmed.startsWith('{')) {
    const parsed = JSON.parse(trimmed)
    if (parsed && typeof parsed === 'object') {
      if (Array.isArray(parsed.questions)) return parsed.questions
      if (Array.isArray(parsed.items)) return parsed.items
      // Single question object
      return [parsed as JsonQuestionItem]
    }
    return []
  }

  // JSONL: one JSON object per line
  const lines = trimmed.split('\n')
  const items: JsonQuestionItem[] = []
  for (const line of lines) {
    const lineTrimmed = line.trim()
    if (!lineTrimmed) continue
    try {
      const parsed = JSON.parse(lineTrimmed)
      if (parsed && typeof parsed === 'object') items.push(parsed)
    } catch {
      // Skip malformed lines
    }
  }
  return items
}

export async function parseJsonQuestionFileToBlocks(file: File): Promise<{
  blocks: ImportBlock[]
  summary: BlockPreviewSummary
}> {
  const text = await file.text()
  const items = parseJsonText(text)

  const blocks: ImportBlock[] = []
  let blocksWithAnomalies = 0
  let questionNumbers = 0
  const anomalyBreakdown: Record<string, number> = {}

  for (let i = 0; i < items.length; i++) {
    const item = items[i]
    const id = crypto.randomUUID()
    const qNum = resolveQuestionNumber(item, i)
    const anomalies = validateJsonQuestionItem(item)
    if (qNum != null) questionNumbers++
    if (anomalies.length > 0) {
      blocksWithAnomalies++
      for (const a of anomalies) {
        anomalyBreakdown[a.code] = (anomalyBreakdown[a.code] || 0) + 1
      }
    }

    blocks.push({
      id,
      index: i,
      original_text: JSON.stringify(item, null, 2),
      current_text: JSON.stringify(item, null, 2),
      question_number: qNum,
      tags: normalizeTags(item.tags),
      metadata: {
        import_format: 'json',
        source_payload: item,
        source_line: i + 1,
      },
      anomalies,
    })
  }

  return {
    blocks,
    summary: {
      total_blocks: blocks.length,
      blocks_with_anomalies: blocksWithAnomalies,
      question_numbers: questionNumbers,
      anomaly_breakdown: anomalyBreakdown,
    },
  }
}
