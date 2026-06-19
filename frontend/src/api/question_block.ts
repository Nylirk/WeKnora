import { post, postUpload } from '@/utils/request'
import { normalizeImportFilePreviewResponse, type ImportFilePreviewResponse } from './question'

// ===== Block Preview / Parse Blocks (two-stage import workbench) =====

export interface ImportBlockAnomaly {
  code: string
  severity: 'error' | 'warning' | 'info'
  message: string
  line?: number
}

export interface ImportBlock {
  id: string
  index: number
  original_text: string
  current_text: string
  question_number: number | null
  tags: string[]
  metadata: Record<string, unknown>
  anomalies: ImportBlockAnomaly[]
}

export interface BlockPreviewSummary {
  total_blocks: number
  blocks_with_anomalies: number
  question_numbers: number
  anomaly_breakdown: Record<string, number>
}

export interface BlockPreviewResponse {
  blocks: ImportBlock[]
  summary: BlockPreviewSummary
}

export interface ParseBlocksRequest {
  blocks: ImportBlock[]
  default_difficulty: string
  strategy_preset: string
}

export type ImportMode = 'single' | 'batch'

export function normalizeBlockPreviewResponse(payload: any): BlockPreviewResponse {
  const source = payload?.data ?? payload ?? {}
  const blocks = Array.isArray(source.blocks) ? source.blocks : []
  const summary = source.summary ?? {}
  return {
    blocks,
    summary: {
      total_blocks: Number(summary.total_blocks ?? blocks.length),
      blocks_with_anomalies: Number(summary.blocks_with_anomalies ?? 0),
      question_numbers: Number(summary.question_numbers ?? 0),
      anomaly_breakdown: summary.anomaly_breakdown ?? {},
    },
  }
}

export const previewImportBlocks = (
  kbId: string,
  setId: string,
  file: File,
  params: { default_difficulty?: string; strategy_preset?: string; import_mode?: string } = {},
  config?: { signal?: AbortSignal; timeout?: number },
): Promise<BlockPreviewResponse> => {
  const fd = new FormData()
  fd.append('file', file)
  const qs = new URLSearchParams()
  if (params.default_difficulty) qs.set('default_difficulty', params.default_difficulty)
  if (params.strategy_preset) qs.set('strategy_preset', params.strategy_preset)
  if (params.import_mode) qs.set('import_mode', params.import_mode)
  const query = qs.toString()
  return postUpload(
    `/api/v1/knowledge-bases/${kbId}/question-sets/${setId}/questions/import-file/block-preview${query ? '?' + query : ''}`,
    fd,
    undefined,
    config,
  ).then((response: any) => normalizeBlockPreviewResponse(response))
}

export const parseImportedBlocks = (
  kbId: string,
  setId: string,
  data: ParseBlocksRequest,
): Promise<ImportFilePreviewResponse> => {
  return post(
    `/api/v1/knowledge-bases/${kbId}/question-sets/${setId}/questions/import-file/parse-blocks`,
    data,
  ).then((response: any) => normalizeImportFilePreviewResponse(response))
}
