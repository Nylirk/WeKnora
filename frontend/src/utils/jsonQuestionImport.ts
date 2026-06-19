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

function parseJsonL(text: string, _fileName?: string): JsonQuestionItem[] {
  const lines = text.split('\n')
  const items: JsonQuestionItem[] = []
  for (let i = 0; i < lines.length; i++) {
    const lineTrimmed = lines[i].trim()
    if (!lineTrimmed) continue
    try {
      const parsed = JSON.parse(lineTrimmed)
      if (parsed && typeof parsed === 'object') {
        items.push(parsed)
      }
    } catch {
      // Generate a warning placeholder for unparseable lines
      items.push({ _jsonl_parse_error: true, _line: i + 1, _raw: lineTrimmed.slice(0, 200) } as unknown as JsonQuestionItem)
    }
  }
  return items
}

function parseJsonText(text: string, fileName?: string): JsonQuestionItem[] {
  const trimmed = text.trim()
  if (!trimmed) return []

  // .jsonl files: always parse line-by-line
  if (fileName && fileName.toLowerCase().endsWith('.jsonl')) {
    return parseJsonL(text, fileName)
  }

  // JSON array
  if (trimmed.startsWith('[')) {
    const parsed = JSON.parse(trimmed)
    if (Array.isArray(parsed)) return parsed.map(item => (typeof item === 'object' && item !== null ? item : {}))
    return []
  }

  // Try JSON object / wrapper; fall back to JSONL on failure
  if (trimmed.startsWith('{')) {
    try {
      const parsed = JSON.parse(trimmed)
      if (parsed && typeof parsed === 'object') {
        if (Array.isArray(parsed.questions)) return parsed.questions
        if (Array.isArray(parsed.items)) return parsed.items
        // Single question object
        return [parsed as JsonQuestionItem]
      }
      return []
    } catch {
      // Not valid JSON — try JSONL
      return parseJsonL(text, fileName)
    }
  }

  // Fallback: try JSONL
  return parseJsonL(text, fileName)
}

export async function parseJsonQuestionFileToBlocks(file: File): Promise<{
  blocks: ImportBlock[]
  summary: BlockPreviewSummary
}> {
  const text = await file.text()
  const items = parseJsonText(text, file.name)

  const blocks: ImportBlock[] = []
  let blocksWithAnomalies = 0
  let questionNumbers = 0
  const anomalyBreakdown: Record<string, number> = {}

  for (let i = 0; i < items.length; i++) {
    const item = items[i]
    const id = crypto.randomUUID()

    // JSONL parse error placeholder
    if ((item as any)._jsonl_parse_error) {
      const lineNum = (item as any)._line || i + 1
      const raw = (item as any)._raw || ''
      const errorAnomaly = { code: 'JSONL_PARSE_ERROR', severity: 'error' as const, message: `第 ${lineNum} 行 JSON 解析失败` }
      blocksWithAnomalies++
      anomalyBreakdown['JSONL_PARSE_ERROR'] = (anomalyBreakdown['JSONL_PARSE_ERROR'] || 0) + 1
      blocks.push({
        id,
        index: i,
        original_text: raw,
        current_text: raw,
        question_number: null,
        tags: [],
        metadata: { import_format: 'json', source_line: lineNum, _parse_error: true },
        anomalies: [errorAnomaly],
      })
      continue
    }

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
