import { toRaw } from 'vue'
import type { ImportBlock } from '@/api/question_block'
import type { ImportQuestionItem } from '@/api/question'
import type { WorkbenchStep } from '@/stores/importWorkbench'
import type { DraftFlushPayload } from '@/stores/importWorkbench'

const DB_NAME = 'question-import-workbench'
const DB_VERSION = 2
const TTL_MS = 7 * 24 * 60 * 60 * 1000

const STORE_META = 'import_draft_meta'
const STORE_BLOCKS = 'import_draft_blocks'
const STORE_QUESTIONS = 'import_draft_questions'

// v1 store name (for migration)
const STORE_V1 = 'drafts'

export interface ImportDraft {
  kbId: string
  setId: string
  blocks?: ImportBlock[]          // v1 compat
  blockOrder?: string[]            // v2
  blockMap?: Record<string, ImportBlock> // v2
  deletedBlockStack?: string[]     // v2
  deletedBlockMap?: Record<string, ImportBlock> // v2
  strategyPreset: string
  defaultDifficulty: string
  importMode: string
  importFormat: string
  currentStep: WorkbenchStep
  questions: ImportQuestionItem[]
  timestamp: number
}

interface DraftMeta {
  kbId: string
  setId: string
  strategyPreset: string
  defaultDifficulty: string
  importMode: string
  importFormat: string
  currentStep: WorkbenchStep
  blockOrder: string[]
  deletedBlockStack: string[]
  updatedAt: number
  expiresAt: number
}

interface BlockRecord {
  draftKey: string
  blockId: string
  block: ImportBlock
  deleted: boolean
}

interface QuestionRecord {
  draftKey: string
  questionIndex: number
  question: ImportQuestionItem
}

function draftKey(kbId: string, setId: string): string {
  return `${kbId}:${setId}`
}

function blockRecordKey(dk: string, blockId: string): string {
  return `${dk}:${blockId}`
}

function questionRecordKey(dk: string, idx: number): string {
  return `${dk}:${idx}`
}

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION)
    request.onupgradeneeded = () => {
      const db = request.result
      // v2 stores
      if (!db.objectStoreNames.contains(STORE_META)) {
        db.createObjectStore(STORE_META)
      }
      if (!db.objectStoreNames.contains(STORE_BLOCKS)) {
        db.createObjectStore(STORE_BLOCKS)
      }
      if (!db.objectStoreNames.contains(STORE_QUESTIONS)) {
        db.createObjectStore(STORE_QUESTIONS)
      }
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

// --- Plain conversion (for IndexedDB-safe serialization) ---

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

function plainBlock(block: unknown): ImportBlock {
  const b = toRaw(block) as any
  return {
    id: typeof b.id === 'string' && b.id ? b.id : crypto.randomUUID(),
    index: typeof b.index === 'number' ? b.index : 0,
    original_text: typeof b.original_text === 'string' ? b.original_text : '',
    current_text: typeof b.current_text === 'string' ? b.current_text : typeof b.original_text === 'string' ? b.original_text : '',
    question_number: typeof b.question_number === 'number' ? b.question_number : null,
    tags: plainStringArray(b.tags),
    metadata: plainObject(b.metadata),
    anomalies: plainAnomalies(b.anomalies),
  }
}

function plainQuestion(item: unknown, index: number): ImportQuestionItem {
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
}

// --- v2: Full save (used on initial parse) ---

export async function saveDraft(draft: ImportDraft): Promise<void> {
  const db = await openDB()
  const dk = draftKey(draft.kbId, draft.setId)
  const now = Date.now()

  return new Promise((resolve, reject) => {
    const tx = db.transaction([STORE_META, STORE_BLOCKS, STORE_QUESTIONS], 'readwrite')

    // Meta
    const meta: DraftMeta = {
      kbId: draft.kbId,
      setId: draft.setId,
      strategyPreset: draft.strategyPreset,
      defaultDifficulty: draft.defaultDifficulty,
      importMode: draft.importMode,
      importFormat: draft.importFormat,
      currentStep: draft.currentStep || 'block-review',
      blockOrder: Array.isArray(draft.blockOrder) ? draft.blockOrder : (Array.isArray(draft.blocks) ? draft.blocks.map(b => b.id) : []),
      deletedBlockStack: Array.isArray(draft.deletedBlockStack) ? draft.deletedBlockStack : [],
      updatedAt: now,
      expiresAt: now + TTL_MS,
    }
    tx.objectStore(STORE_META).put(meta, dk)

    // Blocks — save from blockMap or from blocks array
    const blocksStore = tx.objectStore(STORE_BLOCKS)
    if (draft.blockMap) {
      for (const [blockId, block] of Object.entries(draft.blockMap)) {
        const record: BlockRecord = {
          draftKey: dk,
          blockId,
          block: plainBlock(block),
          deleted: false,
        }
        blocksStore.put(record, blockRecordKey(dk, blockId))
      }
    } else if (Array.isArray(draft.blocks)) {
      for (const block of draft.blocks) {
        const record: BlockRecord = {
          draftKey: dk,
          blockId: block.id,
          block: plainBlock(block),
          deleted: false,
        }
        blocksStore.put(record, blockRecordKey(dk, block.id))
      }
    }

    // Deleted blocks from deletedBlockMap
    if (draft.deletedBlockMap) {
      for (const [blockId, block] of Object.entries(draft.deletedBlockMap)) {
        const record: BlockRecord = {
          draftKey: dk,
          blockId,
          block: plainBlock(block),
          deleted: true,
        }
        blocksStore.put(record, blockRecordKey(dk, blockId))
      }
    }

    // Questions
    const questionsStore = tx.objectStore(STORE_QUESTIONS)
    const questions = Array.isArray(draft.questions) ? draft.questions : []
    for (let i = 0; i < questions.length; i++) {
      const record: QuestionRecord = {
        draftKey: dk,
        questionIndex: i,
        question: plainQuestion(questions[i], i),
      }
      questionsStore.put(record, questionRecordKey(dk, i))
    }

    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

// --- v2: Incremental flush ---

export async function flushDraftChanges(payload: DraftFlushPayload): Promise<void> {
  const db = await openDB()
  const dk = draftKey(payload.kbId, payload.setId)
  const now = Date.now()

  return new Promise((resolve, reject) => {
    const tx = db.transaction([STORE_META, STORE_BLOCKS], 'readwrite')

    // Meta — always update
    const meta: DraftMeta = {
      kbId: payload.kbId,
      setId: payload.setId,
      strategyPreset: payload.strategyPreset,
      defaultDifficulty: payload.defaultDifficulty,
      importMode: payload.importMode,
      importFormat: payload.importFormat,
      currentStep: payload.currentStep,
      blockOrder: payload.blockOrder,
      deletedBlockStack: payload.deletedBlockStack,
      updatedAt: now,
      expiresAt: now + TTL_MS,
    }
    tx.objectStore(STORE_META).put(meta, dk)

    const blocksStore = tx.objectStore(STORE_BLOCKS)

    // Save dirty blocks (active)
    for (const blockId of payload.dirtyBlockIds) {
      const block = payload.blockMap[blockId]
      if (block) {
        const record: BlockRecord = {
          draftKey: dk,
          blockId,
          block: plainBlock(block),
          deleted: false,
        }
        blocksStore.put(record, blockRecordKey(dk, blockId))
      }
    }

    // Handle blocks that moved to deleted
    for (const blockId of payload.deletedBlockStack) {
      const block = payload.deletedBlockMap[blockId]
      if (block && payload.dirtyBlockIds.includes(blockId)) {
        const record: BlockRecord = {
          draftKey: dk,
          blockId,
          block: plainBlock(block),
          deleted: true,
        }
        blocksStore.put(record, blockRecordKey(dk, blockId))
      }
    }

    // Handle blocks that were in deleted but got restored → need to mark as not deleted
    const deletedIds = new Set(payload.deletedBlockStack)
    for (const blockId of payload.dirtyBlockIds) {
      if (!deletedIds.has(blockId) && !payload.blockMap[blockId]) {
        // Block was fully removed (e.g., merged away) — delete from blocks store
        blocksStore.delete(blockRecordKey(dk, blockId))
      }
    }

    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

// --- v2: Load ---

export async function loadDraft(kbId: string, setId: string): Promise<ImportDraft | null> {
  const db = await openDB()
  const dk = draftKey(kbId, setId)

  // Try v2 first
  try {
    const meta = await new Promise<DraftMeta | undefined>((resolve, reject) => {
      const tx = db.transaction(STORE_META, 'readonly')
      const req = tx.objectStore(STORE_META).get(dk)
      req.onsuccess = () => resolve(req.result as DraftMeta | undefined)
      req.onerror = () => reject(req.error)
    })

    if (meta && Date.now() <= meta.expiresAt) {
      // Load blocks
      const blockRecords = await new Promise<BlockRecord[]>((resolve, reject) => {
        const tx = db.transaction(STORE_BLOCKS, 'readonly')
        const req = tx.objectStore(STORE_BLOCKS).getAll()
        req.onsuccess = () => {
          const all = (req.result as BlockRecord[]) || []
          resolve(all.filter(r => r.draftKey === dk))
        }
        req.onerror = () => reject(req.error)
      })

      // Load questions
      const questionRecords = await new Promise<QuestionRecord[]>((resolve, reject) => {
        const tx = db.transaction(STORE_QUESTIONS, 'readonly')
        const req = tx.objectStore(STORE_QUESTIONS).getAll()
        req.onsuccess = () => {
          const all = (req.result as QuestionRecord[]) || []
          resolve(all.filter(r => r.draftKey === dk))
        }
        req.onerror = () => reject(req.error)
      })

      // Rebuild
      const blockMap: Record<string, ImportBlock> = {}
      const deletedBlockMap: Record<string, ImportBlock> = {}
      for (const record of blockRecords) {
        if (record.deleted) {
          deletedBlockMap[record.blockId] = record.block
        } else {
          blockMap[record.blockId] = record.block
        }
      }

      const questions = questionRecords
        .sort((a, b) => a.questionIndex - b.questionIndex)
        .map(r => r.question)

      return {
        kbId: meta.kbId,
        setId: meta.setId,
        blockOrder: meta.blockOrder,
        blockMap,
        deletedBlockStack: meta.deletedBlockStack || [],
        deletedBlockMap,
        strategyPreset: meta.strategyPreset,
        defaultDifficulty: meta.defaultDifficulty,
        importMode: meta.importMode,
        importFormat: meta.importFormat,
        currentStep: meta.currentStep,
        questions,
        timestamp: meta.updatedAt,
      }
    }
  } catch {
    // Fall through to v1 migration
  }

  // P6: Try v1 migration
  if (db.objectStoreNames.contains(STORE_V1)) {
    try {
      const oldDraft = await new Promise<any>((resolve, reject) => {
        const tx = db.transaction(STORE_V1, 'readonly')
        const req = tx.objectStore(STORE_V1).get(dk)
        req.onsuccess = () => resolve(req.result)
        req.onerror = () => reject(req.error)
      })

      if (oldDraft && Date.now() - oldDraft.timestamp <= TTL_MS) {
        // Migrate to v2
        const migrated: ImportDraft = {
          kbId: oldDraft.kbId || kbId,
          setId: oldDraft.setId || setId,
          blocks: oldDraft.blocks || [],
          blockOrder: (oldDraft.blocks || []).map((b: ImportBlock) => b.id),
          blockMap: {},
          deletedBlockStack: [],
          deletedBlockMap: {},
          strategyPreset: oldDraft.strategyPreset || 'general',
          defaultDifficulty: oldDraft.defaultDifficulty || 'medium',
          importMode: oldDraft.importMode || 'batch',
          importFormat: oldDraft.importFormat || 'word',
          currentStep: oldDraft.currentStep || 'block-review',
          questions: oldDraft.questions || [],
          timestamp: oldDraft.timestamp || Date.now(),
        }
        // Build blockMap
        for (const b of (oldDraft.blocks || [])) {
          migrated.blockMap![b.id] = b
        }
        // Write back in v2 format
        await saveDraft(migrated)
        // Delete old v1 record
        try {
          await new Promise<void>((resolve) => {
            const tx = db.transaction(STORE_V1, 'readwrite')
            tx.objectStore(STORE_V1).delete(dk)
            tx.oncomplete = () => resolve()
            tx.onerror = () => resolve()
          })
        } catch { /* best-effort */ }
        return migrated
      }
    } catch {
      // v1 not found or expired
    }
  }

  return null
}

// --- v2: Delete ---

export async function deleteDraft(kbId: string, setId: string): Promise<void> {
  const db = await openDB()
  const dk = draftKey(kbId, setId)

  return new Promise((resolve, reject) => {
    const tx = db.transaction([STORE_META, STORE_BLOCKS, STORE_QUESTIONS], 'readwrite')

    // Delete meta
    tx.objectStore(STORE_META).delete(dk)

    // Delete all blocks for this draft key
    const blocksStore = tx.objectStore(STORE_BLOCKS)
    const blockReq = blocksStore.getAll()
    blockReq.onsuccess = () => {
      const records = (blockReq.result as BlockRecord[]) || []
      for (const r of records) {
        if (r.draftKey === dk) {
          blocksStore.delete(blockRecordKey(dk, r.blockId))
        }
      }
    }

    // Delete all questions for this draft key
    const questionsStore = tx.objectStore(STORE_QUESTIONS)
    const questionReq = questionsStore.getAll()
    questionReq.onsuccess = () => {
      const records = (questionReq.result as QuestionRecord[]) || []
      for (const r of records) {
        if (r.draftKey === dk) {
          questionsStore.delete(questionRecordKey(dk, r.questionIndex))
        }
      }
    }

    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

// --- v2: Clean expired ---

export async function cleanExpiredDrafts(): Promise<void> {
  const db = await openDB()
  const now = Date.now()

  return new Promise((resolve, reject) => {
    const tx = db.transaction([STORE_META, STORE_BLOCKS, STORE_QUESTIONS], 'readwrite')
    const metaStore = tx.objectStore(STORE_META)
    const blocksStore = tx.objectStore(STORE_BLOCKS)
    const questionsStore = tx.objectStore(STORE_QUESTIONS)

    const req = metaStore.getAll()
    req.onsuccess = () => {
      const allMeta = (req.result as DraftMeta[]) || []
      const expiredKeys: string[] = []
      for (const meta of allMeta) {
        if (now > meta.expiresAt) {
          expiredKeys.push(draftKey(meta.kbId, meta.setId))
          metaStore.delete(draftKey(meta.kbId, meta.setId))
        }
      }

      // Clean blocks and questions for expired drafts
      const blockReq = blocksStore.getAll()
      blockReq.onsuccess = () => {
        const blockRecords = (blockReq.result as BlockRecord[]) || []
        for (const r of blockRecords) {
          if (expiredKeys.includes(r.draftKey)) {
            blocksStore.delete(blockRecordKey(r.draftKey, r.blockId))
          }
        }
      }
      const questionReq = questionsStore.getAll()
      questionReq.onsuccess = () => {
        const questionRecords = (questionReq.result as QuestionRecord[]) || []
        for (const r of questionRecords) {
          if (expiredKeys.includes(r.draftKey)) {
            questionsStore.delete(questionRecordKey(r.draftKey, r.questionIndex))
          }
        }
      }
    }

    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

// --- Register flush handler ---

import { registerDraftFlushHandler } from '@/stores/importWorkbench'

registerDraftFlushHandler(async (payload: DraftFlushPayload) => {
  await flushDraftChanges(payload)
})
