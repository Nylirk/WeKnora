import { toRaw } from 'vue'
import type { ImportBlock } from '@/api/question_block'
import type { ImportQuestionItem } from '@/api/question'
import type { WorkbenchStep } from '@/stores/importWorkbench'

const DB_NAME = 'question-import-workbench'
const DB_VERSION = 1
const STORE_NAME = 'drafts'
const TTL_MS = 7 * 24 * 60 * 60 * 1000

export interface ImportDraft {
  kbId: string
  setId: string
  blocks: ImportBlock[]
  strategyPreset: string
  defaultDifficulty: string
  importMode: string
  importFormat: string
  currentStep: WorkbenchStep
  questions: ImportQuestionItem[]
  timestamp: number
}

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION)
    request.onupgradeneeded = () => {
      const db = request.result
      if (!db.objectStoreNames.contains(STORE_NAME)) {
        db.createObjectStore(STORE_NAME)
      }
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

function draftKey(kbId: string, setId: string): string { return `${kbId}:${setId}` }

// --- Plain conversion: strip Vue reactive proxies for IndexedDB storage ---

function plainJsonValue(value: unknown): unknown {
  const raw = toRaw(value)
  if (raw === null || typeof raw === 'string' || typeof raw === 'number' || typeof raw === 'boolean') return raw
  if (Array.isArray(raw)) return raw.map(plainJsonValue)
  if (typeof raw === 'object') {
    const out: Record<string, unknown> = {}
    for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
      if (typeof v === 'function' || typeof v === 'symbol' || typeof v === 'undefined') continue
      out[k] = plainJsonValue(v)
    }
    return out
  }
  return String(raw)
}

function plainStringArray(input: unknown): string[] {
  const raw = toRaw(input)
  if (!Array.isArray(raw)) return []
  const seen = new Set<string>()
  const out: string[] = []
  for (const item of raw) {
    const value = String(item ?? '').trim()
    if (!value || seen.has(value)) continue
    seen.add(value)
    out.push(value)
  }
  return out
}

function plainAnomalies(input: unknown) {
  const raw = toRaw(input)
  if (!Array.isArray(raw)) return []
  return raw.map((item: any) => {
    const a = toRaw(item) as any
    return {
      code: String(a?.code || ''),
      severity: a?.severity === 'error' || a?.severity === 'warning' || a?.severity === 'info' ? a.severity : 'warning',
      message: String(a?.message || ''),
      line: typeof a?.line === 'number' ? a.line : undefined,
    }
  }).filter((a: any) => a.code)
}

function plainObject(input: unknown): Record<string, unknown> {
  const raw = toRaw(input)
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return {}
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(raw as Record<string, unknown>)) {
    if (typeof value === 'function' || typeof value === 'symbol' || typeof value === 'undefined') continue
    out[key] = plainJsonValue(value)
  }
  return out
}

function plainBlocks(input: unknown): ImportBlock[] {
  const raw = toRaw(input)
  if (!Array.isArray(raw)) return []
  return raw.map((block: any, index: number) => {
    const b = toRaw(block) as any
    return {
      id: typeof b.id === 'string' && b.id ? b.id : crypto.randomUUID(),
      index: typeof b.index === 'number' ? b.index : index,
      original_text: typeof b.original_text === 'string' ? b.original_text : '',
      current_text: typeof b.current_text === 'string' ? b.current_text : typeof b.original_text === 'string' ? b.original_text : '',
      question_number: typeof b.question_number === 'number' ? b.question_number : null,
      tags: plainStringArray(b.tags),
      metadata: plainObject(b.metadata),
      anomalies: plainAnomalies(b.anomalies),
    }
  })
}

function plainQuestions(input: unknown): ImportQuestionItem[] {
  const raw = toRaw(input)
  if (!Array.isArray(raw)) return []
  return raw.map((item: any, index: number) => {
    const q = toRaw(item) as any
    return {
      line_number: typeof q.line_number === 'number' ? q.line_number : index + 1,
      question_type: typeof q.question_type === 'string' ? q.question_type : 'short_answer',
      stem_text: typeof q.stem_text === 'string' ? q.stem_text : '',
      question_body: typeof q.question_body === 'object' && q.question_body ? plainObject(q.question_body as unknown) : {},
      answer_text: typeof q.answer_text === 'string' ? q.answer_text : '',
      answer_body: typeof q.answer_body === 'object' && q.answer_body ? plainObject(q.answer_body as unknown) : {},
      analysis_text: typeof q.analysis_text === 'string' ? q.analysis_text : '',
      grading_rubric: typeof q.grading_rubric === 'object' && q.grading_rubric ? plainObject(q.grading_rubric as unknown) : {},
      difficulty: typeof q.difficulty === 'string' ? q.difficulty : 'medium',
      knowledge_points: Array.isArray(q.knowledge_points) ? q.knowledge_points.map((kp: unknown) => String(kp ?? '')) : [],
      tags: Array.isArray(q.tags) ? q.tags.map((t: unknown) => String(t ?? '')) : [],
      source_knowledge_id: typeof q.source_knowledge_id === 'string' ? q.source_knowledge_id : '',
      evidence_chunk_ids: Array.isArray(q.evidence_chunk_ids) ? q.evidence_chunk_ids.map((id: unknown) => String(id ?? '')) : [],
      status: typeof q.status === 'string' ? q.status : 'draft',
      raw_text: typeof q.raw_text === 'string' ? q.raw_text : '',
      source_payload: typeof q.source_payload === 'object' && q.source_payload ? plainObject(q.source_payload as unknown) : {},
    }
  })
}

function toPlainDraft(input: ImportDraft): ImportDraft {
  return {
    kbId: String(input.kbId || ''),
    setId: String(input.setId || ''),
    blocks: plainBlocks(input.blocks),
    strategyPreset: String(input.strategyPreset || 'general'),
    defaultDifficulty: String(input.defaultDifficulty || 'medium'),
    importMode: input.importMode === 'batch' ? 'batch' : 'single',
    importFormat: input.importFormat === 'pdf' ? 'pdf' : input.importFormat === 'json' ? 'json' : 'word',
    currentStep: input.currentStep === 'question-review' ? 'question-review' : 'block-review',
    questions: plainQuestions(input.questions || []),
    timestamp: Number(input.timestamp || Date.now()),
  }
}

// --- Public API ---

export async function saveDraft(draft: ImportDraft): Promise<void> {
  const plainDraft = toPlainDraft(draft)
  plainDraft.timestamp = Date.now()
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    const key = draftKey(plainDraft.kbId, plainDraft.setId)
    store.put(plainDraft, key)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

export async function loadDraft(kbId: string, setId: string): Promise<ImportDraft | null> {
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readonly')
    const store = tx.objectStore(STORE_NAME)
    const key = draftKey(kbId, setId)
    const request = store.get(key)
    request.onsuccess = () => {
      const draft = request.result as ImportDraft | undefined
      if (!draft) { resolve(null); return }
      if (Date.now() - draft.timestamp > TTL_MS) {
        deleteDraft(kbId, setId).catch(() => {})
        resolve(null)
        return
      }
      resolve(draft)
    }
    request.onerror = () => reject(request.error)
  })
}

export async function deleteDraft(kbId: string, setId: string): Promise<void> {
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    store.delete(draftKey(kbId, setId))
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

export async function cleanExpiredDrafts(): Promise<void> {
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    const request = store.getAll()
    request.onsuccess = () => {
      const drafts = request.result as ImportDraft[]
      const now = Date.now()
      for (const draft of drafts) {
        if (now - draft.timestamp > TTL_MS) {
          store.delete(draftKey(draft.kbId, draft.setId))
        }
      }
      tx.oncomplete = () => resolve()
      tx.onerror = () => reject(tx.error)
    }
    request.onerror = () => reject(request.error)
  })
}
