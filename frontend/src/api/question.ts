import { del, get, post, postUpload, put } from '@/utils/request'
import type { PageResult } from './evaluation'

export type QuestionType = 'single_choice' | 'multiple_choice' | 'true_false' | 'fill_blank' | 'short_answer' | 'essay' | 'composite'
export type QuestionDifficulty = 'easy' | 'medium' | 'hard'
export type QuestionSetSourceType = 'manual' | 'import' | 'generated' | 'exam_paper'
export type QuestionSetStatus = 'active' | 'completed' | 'pending' | 'failed'
export type QuestionStatus = 'draft' | 'reviewed' | 'rejected'

export interface QuestionOption { label: string; content: string }
export interface ChoiceQuestionBody { options: QuestionOption[]; min_select?: number; max_select?: number }
export interface SingleChoiceAnswer { selected_index: number; explanation?: string }
export interface MultipleChoiceAnswer { selected_indices: number[]; explanation?: string }
export interface TrueFalseAnswer { is_true: boolean; explanation?: string }
export interface FillBlankAnswer { blank_answers: string[] }
export interface ShortAnswerAnswer { keywords?: string[]; explanation?: string }

export type QuestionSetProcessingStage = '' | 'draft_imported' | 'indexing' | 'auto_tagging' | 'syllabus_checking' | 'ready_for_review' | 'failed'

export interface ProcessingStageDetail {
  key: string
  label: string
  status: 'completed' | 'running' | 'paused' | 'failed' | 'pending'
  reason?: string
}

export interface QuestionSetProcessingStatus {
  stage: QuestionSetProcessingStage
  error_message: string
  skipped_auto_tagging_reason?: string
  skipped_syllabus_reason?: string
  auto_tagging_enabled: boolean
  syllabus_check_enabled: boolean
  stages?: ProcessingStageDetail[]
}

// ProcessingButtonState models the toolbar button's visual/behavioural mode.
export type ProcessingButtonState = 'hidden' | 'running' | 'paused' | 'failed' | 'ready_for_review' | 'completed'

export const PROCESSING_STAGE_LABELS: Record<string, string> = {
  draft_imported: '导入完成',
  indexing: '索引处理',
  auto_tagging: '知识点关联',
  syllabus_checking: '考纲筛选',
  ready_for_review: '待人工审核',
}

export const PROCESSING_STAGE_STATUS_LABELS: Record<string, string> = {
  completed: '已完成',
  running: '进行中',
  paused: '暂停',
  failed: '失败',
  pending: '待处理',
}

export const PROCESSING_BUTTON_LABELS: Record<ProcessingButtonState, string> = {
  hidden: '',
  running: '处理中',
  paused: '部分暂停',
  failed: '处理失败',
  ready_for_review: '待人工审核',
  completed: '处理完成',
}

const STAGE_ORDER = ['draft_imported', 'indexing', 'auto_tagging', 'syllabus_checking']

/**
 * Derive per-stage status from the API response.
 * Prefer backend-computed stages when available; fall back to local derivation.
 */
export function resolveProcessingStages(status: QuestionSetProcessingStatus): ProcessingStageDetail[] {
  // When backend provides structured stages, use them directly (filter out ready_for_review).
  if (status.stages && status.stages.length > 0) {
    return status.stages.filter(s => s.key !== 'ready_for_review')
  }

  // Fallback: derive from current stage + config booleans.
  const currentStage = status.stage
  const stages: ProcessingStageDetail[] = STAGE_ORDER.map(key => ({
    key,
    label: PROCESSING_STAGE_LABELS[key] || key,
    status: 'pending' as const,
  }))

  if (!currentStage) return stages

  const currentIdx = STAGE_ORDER.indexOf(currentStage)
  const isFailed = currentStage === 'failed'
  const isReadyForReview = currentStage === 'ready_for_review'

  // ready_for_review is a terminal status: all 4 stages are completed.
  if (isReadyForReview) {
    for (let i = 0; i < stages.length; i++) {
      stages[i].status = 'completed'
    }
  } else {
    for (let i = 0; i < stages.length; i++) {
      if (i < currentIdx) {
        stages[i].status = 'completed'
      } else if (i === currentIdx && !isFailed) {
        stages[i].status = 'running'
      }
    }
  }

  if (isFailed) {
    // Mark prior stages completed and the next expected stage as failed.
    for (let i = 0; i < stages.length; i++) {
      if (stages[i].status === 'pending') {
        stages[i].status = 'failed'
        break
      }
    }
  }

  // Paused stages from missing config (override only when not running).
  const autoIdx = stages.findIndex(s => s.key === 'auto_tagging')
  const syllabusIdx = stages.findIndex(s => s.key === 'syllabus_checking')
  if (!status.auto_tagging_enabled && status.skipped_auto_tagging_reason && autoIdx >= 0 && stages[autoIdx].status !== 'running') {
    stages[autoIdx].status = 'paused'
    stages[autoIdx].reason = status.skipped_auto_tagging_reason
  }
  if (!status.syllabus_check_enabled && status.skipped_syllabus_reason && syllabusIdx >= 0 && stages[syllabusIdx].status !== 'running') {
    stages[syllabusIdx].status = 'paused'
    stages[syllabusIdx].reason = status.skipped_syllabus_reason
  }

  return stages
}

/**
 * Resolve the toolbar button state from the processing status.
 * Priority: failed > running > paused > ready_for_review/completed > hidden
 */
export function resolveProcessingButtonState(status: QuestionSetProcessingStatus | null): {
  state: ProcessingButtonState
  completedCount: number
  totalCount: number
} {
  if (!status || !status.stage) {
    return { state: 'hidden', completedCount: 0, totalCount: 0 }
  }

  const stages = resolveProcessingStages(status)
  const completedCount = stages.filter(s => s.status === 'completed').length
  const totalCount = stages.length

  // 1. failed – any stage failed or overall stage is failed
  if (status.stage === 'failed' || stages.some(s => s.status === 'failed')) {
    return { state: 'failed', completedCount, totalCount }
  }

  // 2. running – any stage currently running
  if (stages.some(s => s.status === 'running')) {
    return { state: 'running', completedCount, totalCount }
  }

  // 3. paused – any stage paused (config missing)
  if (stages.some(s => s.status === 'paused')) {
    return { state: 'paused', completedCount, totalCount }
  }

  // 4. ready_for_review
  if (status.stage === 'ready_for_review') {
    return { state: 'ready_for_review', completedCount, totalCount }
  }

  return { state: 'completed', completedCount, totalCount }
}

export interface QuestionSet {
  id: string; tenant_id: number; knowledge_base_id: string
  name: string; description: string; source_type: QuestionSetSourceType
  status: QuestionSetStatus; question_count: number
  generation_config: Record<string, unknown>; generation_scope: Record<string, unknown>
  processing_stage: QuestionSetProcessingStage
  error_message: string; created_at: string; updated_at: string
}

export interface QuestionBankConfig {
  knowledge_point_knowledge_base_id: string
  syllabus_knowledge_base_id: string
}

export interface Question {
  id: string; tenant_id: number; question_set_id: string; knowledge_base_id: string
  question_type: QuestionType; schema_version: string
  stem_text: string; question_body: Record<string, unknown>
  answer_text: string; answer_body: Record<string, unknown>
  analysis_text: string; grading_rubric: Record<string, unknown>
  difficulty: QuestionDifficulty; status: QuestionStatus
  knowledge_points: string[]; tags: string[]
  source_knowledge_id: string; evidence_chunk_ids: string[]
  source_payload: Record<string, unknown>; extraction_metadata: Record<string, unknown>
  sort_order: number; created_at: string; updated_at: string
  reviewed_by?: string; reviewed_at?: string
}

export interface QuestionListFilter {
  question_type?: QuestionType; difficulty?: QuestionDifficulty
  status?: QuestionStatus; knowledge_point?: string; tag?: string; keyword?: string
}

export interface ImportQuestionItem {
  line_number: number; question_type: QuestionType
  stem_text: string; question_body: Record<string, unknown>
  answer_text: string; answer_body: Record<string, unknown>
  analysis_text: string; grading_rubric: Record<string, unknown>
  difficulty: QuestionDifficulty; knowledge_points: string[]; tags: string[]
  source_knowledge_id: string; evidence_chunk_ids: string[]
  status?: QuestionStatus; raw_text?: string
}

export interface ImportQuestionError { line_number: number; message: string }
export interface ImportQuestionsRequest { items: ImportQuestionItem[] }
export interface ImportQuestionsResult { created: number; errors: ImportQuestionError[] }

const unwrap = <T>(response: any): T => response.data as T

export const listQuestionSets = (kbId: string, page = 1, pageSize = 50) =>
  get(`/api/v1/knowledge-bases/${kbId}/question-sets?page=${page}&page_size=${pageSize}`).then(unwrap<PageResult<QuestionSet>>)

export const createQuestionSet = (kbId: string, data: { name: string; description?: string }) =>
  post(`/api/v1/knowledge-bases/${kbId}/question-sets`, data).then(unwrap<QuestionSet>)

export const getQuestionSet = (kbId: string, setId: string) =>
  get(`/api/v1/knowledge-bases/${kbId}/question-sets/${setId}`).then(unwrap<QuestionSet>)

export const updateQuestionSet = (kbId: string, setId: string, data: Partial<{ name: string; description: string; status: string }>) =>
  put(`/api/v1/knowledge-bases/${kbId}/question-sets/${setId}`, data).then(unwrap<QuestionSet>)

export const deleteQuestionSet = (kbId: string, setId: string) =>
  del(`/api/v1/knowledge-bases/${kbId}/question-sets/${setId}`)

export const generateQuestions = (kbId: string, data: { name: string; description?: string; generation_config?: Record<string, unknown>; generation_scope?: Record<string, unknown> }) =>
  post(`/api/v1/knowledge-bases/${kbId}/question-sets/generate`, data).then(unwrap<QuestionSet>)

export const listQuestions = (kbId: string, setId: string, filter?: QuestionListFilter, page = 1, pageSize = 50) => {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
  if (filter) {
    if (filter.question_type) params.set('question_type', filter.question_type)
    if (filter.difficulty) params.set('difficulty', filter.difficulty)
    if (filter.status) params.set('status', filter.status)
    if (filter.keyword) params.set('keyword', filter.keyword)
    if (filter.knowledge_point) params.set('knowledge_point', filter.knowledge_point)
    if (filter.tag) params.set('tag', filter.tag)
  }
  return get(`/api/v1/knowledge-bases/${kbId}/question-sets/${setId}/questions?${params}`).then(unwrap<PageResult<Question>>)
}

export const createQuestion = (kbId: string, setId: string, data: Record<string, unknown>) =>
  post(`/api/v1/knowledge-bases/${kbId}/question-sets/${setId}/questions`, data).then(unwrap<Question>)

export const getQuestion = (kbId: string, setId: string, questionId: string) =>
  get(`/api/v1/knowledge-bases/${kbId}/question-sets/${setId}/questions/${questionId}`).then(unwrap<Question>)

export const updateQuestion = (kbId: string, setId: string, questionId: string, data: Record<string, unknown>) =>
  put(`/api/v1/knowledge-bases/${kbId}/question-sets/${setId}/questions/${questionId}`, data).then(unwrap<Question>)

export const deleteQuestion = (kbId: string, setId: string, questionId: string) =>
  del(`/api/v1/knowledge-bases/${kbId}/question-sets/${setId}/questions/${questionId}`)

export const updateQuestionStatus = (kbId: string, setId: string, questionId: string, data: { status: string }) =>
  put(`/api/v1/knowledge-bases/${kbId}/question-sets/${setId}/questions/${questionId}/status`, data).then(unwrap<Question>)

export const importQuestions = (kbId: string, setId: string, data: ImportQuestionsRequest) =>
  post(`/api/v1/knowledge-bases/${kbId}/question-sets/${setId}/questions/import`, data).then(unwrap<ImportQuestionsResult>)

export const getQuestionSetProcessingStatus = (kbId: string, setId: string) =>
  get(`/api/v1/knowledge-bases/${kbId}/question-sets/${setId}/processing-status`).then(unwrap<QuestionSetProcessingStatus>)

export type QuestionProcessingReprocessScope = 'all' | 'auto_tagging' | 'syllabus_checking'

/** Trigger reprocessing of draft questions in the given scope. */
export const reprocessQuestionSet = (kbId: string, setId: string, scope: QuestionProcessingReprocessScope) =>
  post(`/api/v1/knowledge-bases/${kbId}/question-sets/${setId}/processing/reprocess`, { scope })

export const exportToEvaluationDataset = (kbId: string, setId: string, data: { name: string; description?: string }) =>
  post(`/api/v1/knowledge-bases/${kbId}/question-sets/${setId}/questions/export`, data).then(unwrap<any>)

export interface ImportFilePreviewStats {
  detected_questions: number
  with_answer: number
  without_answer: number
}

export interface ImportFilePreviewResponse {
  items: ImportQuestionItem[]
  errors: ImportQuestionError[]
  warnings: string[]
  raw_text_preview: string
  stats: ImportFilePreviewStats
}

export function normalizeImportFilePreviewResponse(payload: any): ImportFilePreviewResponse {
  // Unwrap common response shapes: { data: ... }, { data: { data: ... } }
  const source = payload?.data?.data ?? payload?.data ?? payload ?? {}

  const items = Array.isArray(source.items) ? source.items : []
  const errors = Array.isArray(source.errors) ? source.errors : []
  const warnings = Array.isArray(source.warnings) ? source.warnings : []
  const rawText =
    typeof source.raw_text_preview === 'string'
      ? source.raw_text_preview
      : typeof source.rawTextPreview === 'string'
        ? source.rawTextPreview
        : ''

  return {
    items,
    errors,
    warnings,
    raw_text_preview: rawText,
    stats: {
      detected_questions: Number(
        source.stats?.detected_questions ?? source.stats?.detectedQuestions ?? items.length,
      ),
      with_answer: Number(
        source.stats?.with_answer ??
          source.stats?.withAnswer ??
          items.filter((item: any) => !!String(item.answer_text || '').trim()).length,
      ),
      without_answer: Number(
        source.stats?.without_answer ??
          source.stats?.withoutAnswer ??
          items.filter((item: any) => !String(item.answer_text || '').trim()).length,
      ),
    },
  }
}

export const previewImportFile = (
  kbId: string,
  setId: string,
  file: File,
  params: { default_question_type?: string; default_difficulty?: string; mode?: string } = {},
  config?: { signal?: AbortSignal; timeout?: number },
): Promise<ImportFilePreviewResponse> => {
  const fd = new FormData()
  fd.append('file', file)
  // Query params for the handler's ShouldBindQuery
  const qs = new URLSearchParams()
  if (params.default_question_type) qs.set('default_question_type', params.default_question_type)
  if (params.default_difficulty) qs.set('default_difficulty', params.default_difficulty)
  if (params.mode) qs.set('mode', params.mode)
  const query = qs.toString()
  return postUpload(
    `/api/v1/knowledge-bases/${kbId}/question-sets/${setId}/questions/import-file/preview${query ? '?' + query : ''}`,
    fd,
    undefined,
    config,
  ).then((response: any) => normalizeImportFilePreviewResponse(response))
}

// --- Syllabus management for question bank KBs ---

export interface SyllabusInfo {
  syllabus_kb_id: string
  file_name: string
  file_size: number
  parse_status: string
  knowledge_count: number
  chunk_count: number
  created_at: string
  updated_at: string
}

export interface SyllabusUploadResponse {
  syllabus_kb_id: string
  file_name: string
  parse_status: string
  knowledge_count: number
  chunk_count: number
  message: string
}

/** Upload a syllabus file for a question bank KB. */
export function uploadSyllabus(kbId: string, file: File): Promise<SyllabusUploadResponse> {
  const fd = new FormData()
  fd.append('file', file)
  return postUpload(`/api/v1/knowledge-bases/${kbId}/question-bank/syllabus`, fd).then(
    (r: any) => r?.data ?? r,
  )
}

/** Get syllabus info for a question bank KB. */
export function getSyllabus(kbId: string): Promise<SyllabusInfo | null> {
  return get(`/api/v1/knowledge-bases/${kbId}/question-bank/syllabus`).then(
    (r: any) => r?.data ?? null,
  )
}

/** Delete the syllabus from a question bank KB. */
export function deleteSyllabus(kbId: string): Promise<void> {
  return del(`/api/v1/knowledge-bases/${kbId}/question-bank/syllabus`)
}