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

export interface QuestionSet {
  id: string; tenant_id: number; knowledge_base_id: string
  name: string; description: string; source_type: QuestionSetSourceType
  status: QuestionSetStatus; question_count: number
  generation_config: Record<string, unknown>; generation_scope: Record<string, unknown>
  error_message: string; created_at: string; updated_at: string
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
  status?: QuestionStatus
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
  ).then(unwrap<ImportFilePreviewResponse>)
}