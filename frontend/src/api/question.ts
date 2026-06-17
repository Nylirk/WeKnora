import { del, get, post, put } from '@/utils/request'
import type { PageResult } from './evaluation'

export type QuestionType = 'single_choice' | 'multiple_choice' | 'true_false' | 'fill_blank' | 'short_answer' | 'essay' | 'composite'
export type QuestionDifficulty = 'easy' | 'medium' | 'hard'
export type QuestionSetSourceType = 'manual' | 'imported' | 'generated'
export type QuestionSetStatus = 'active' | 'archived' | 'pending'
export type QuestionStatus = 'draft' | 'reviewed' | 'rejected'

export interface QuestionOption { label: string; content: string }
export interface SingleChoiceAnswer { selected_index: number; explanation?: string }
export interface MultipleChoiceAnswer { selected_indices: number[]; explanation?: string }
export interface TrueFalseAnswer { is_true: boolean; explanation?: string }
export interface FillBlankAnswer { blank_answers: string[] }
export interface ShortAnswerAnswer { keywords?: string[]; explanation?: string }

export interface QuestionSet {
  id: string; tenant_id: number; knowledge_base_id: string
  name: string; description: string; source_type: QuestionSetSourceType
  status: QuestionSetStatus; question_count: number
  generation_config: Record<string, unknown>; metadata: Record<string, unknown>
  created_at: string; updated_at: string
}

export interface Question {
  id: string; tenant_id: number; question_set_id: string
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
}

export interface ImportQuestionError { line_number: number; message: string }
export interface ImportQuestionsRequest { items: ImportQuestionItem[] }
export interface ImportQuestionsResult { created: number; errors: ImportQuestionError[] }

const unwrap = <T>(response: any): T => response.data as T

export const listQuestionSets = (kbId: string, page = 1, pageSize = 50) =>
  get(`/api/v1/knowledge-bases/${kbId}/question-sets?page=${page}&page_size=${pageSize}`).then(unwrap<PageResult<QuestionSet>>)

export const createQuestionSet = (kbId: string, data: { name: string; description?: string; knowledge_base_id: string }) =>
  post(`/api/v1/knowledge-bases/${kbId}/question-sets`, data).then(unwrap<QuestionSet>)

export const getQuestionSet = (setId: string) =>
  get(`/api/v1/question-sets/${setId}`).then(unwrap<QuestionSet>)

export const updateQuestionSet = (setId: string, data: Partial<{ name: string; description: string; status: string }>) =>
  put(`/api/v1/question-sets/${setId}`, data).then(unwrap<QuestionSet>)

export const deleteQuestionSet = (setId: string) =>
  del(`/api/v1/question-sets/${setId}`)

export const listQuestions = (setId: string, filter?: QuestionListFilter, page = 1, pageSize = 50) => {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
  if (filter) {
    if (filter.question_type) params.set('question_type', filter.question_type)
    if (filter.difficulty) params.set('difficulty', filter.difficulty)
    if (filter.status) params.set('status', filter.status)
    if (filter.keyword) params.set('keyword', filter.keyword)
    if (filter.knowledge_point) params.set('knowledge_point', filter.knowledge_point)
    if (filter.tag) params.set('tag', filter.tag)
  }
  return get(`/api/v1/question-sets/${setId}/questions?${params}`).then(unwrap<PageResult<Question>>)
}

export const createQuestion = (setId: string, data: Record<string, unknown>) =>
  post(`/api/v1/question-sets/${setId}/questions`, data).then(unwrap<Question>)

export const getQuestion = (setId: string, questionId: string) =>
  get(`/api/v1/question-sets/${setId}/questions/${questionId}`).then(unwrap<Question>)

export const updateQuestion = (setId: string, questionId: string, data: Record<string, unknown>) =>
  put(`/api/v1/question-sets/${setId}/questions/${questionId}`, data).then(unwrap<Question>)

export const deleteQuestion = (setId: string, questionId: string) =>
  del(`/api/v1/question-sets/${setId}/questions/${questionId}`)

export const updateQuestionStatus = (setId: string, questionId: string, data: { status: string }) =>
  put(`/api/v1/question-sets/${setId}/questions/${questionId}/status`, data).then(unwrap<Question>)

export const importQuestions = (setId: string, data: ImportQuestionsRequest) =>
  post(`/api/v1/question-sets/${setId}/questions/import`, data).then(unwrap<ImportQuestionsResult>)

export const exportToEvaluationDataset = (setId: string, data: { name: string; description?: string }) =>
  post(`/api/v1/question-sets/${setId}/questions/export`, data).then(unwrap<any>)

export const generateQuestions = (kbId: string, data: { name: string; description?: string; knowledge_base_id: string; generation_config?: Record<string, unknown> }) =>
  post(`/api/v1/knowledge-bases/${kbId}/question-sets/generate`, data).then(unwrap<QuestionSet>)