import { defineStore } from 'pinia'
import { ref, computed, shallowRef } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import type { ImportBlock } from '@/api/question_block'
import type { ImportQuestionItem } from '@/api/question'
import { parseImportedBlocks } from '@/api/question_block'

export type WorkbenchStep = 'block-review' | 'question-review'

/** Structural anomaly codes recomputed from block ordering — kept separate from block.anomalies */
export const STRUCTURAL_CODES = new Set([
  'DUPLICATE_QUESTION_NUMBER',
  'NON_MONOTONIC_QUESTION_NUMBER',
  'QUESTION_NUMBER_GAP',
  'MISSING_QUESTION_NUMBER',
])

export function normalizeTags(tags: unknown): string[] {
  if (!Array.isArray(tags)) return []
  const seen = new Set<string>()
  const result: string[] = []
  for (const t of tags) {
    if (typeof t !== 'string') continue
    const trimmed = t.trim()
    if (!trimmed) continue
    if (seen.has(trimmed)) continue
    seen.add(trimmed)
    result.push(trimmed)
  }
  return result
}

export function normalizeImportBlock(block: ImportBlock, index: number): ImportBlock {
  return {
    ...block,
    id: typeof block.id === 'string' && block.id ? block.id : crypto.randomUUID(),
    index: typeof block.index === 'number' ? block.index : index,
    original_text: typeof block.original_text === 'string' ? block.original_text : '',
    current_text: typeof block.current_text === 'string'
      ? block.current_text
      : typeof block.original_text === 'string'
        ? block.original_text
        : '',
    question_number: typeof block.question_number === 'number' ? block.question_number : null,
    tags: normalizeTags(block.tags),
    metadata: block.metadata && typeof block.metadata === 'object' && !Array.isArray(block.metadata) ? block.metadata : {},
    anomalies: Array.isArray(block.anomalies) ? block.anomalies : [],
  }
}

export function normalizeImportBlocks(blocks: ImportBlock[] | null | undefined): ImportBlock[] {
  return Array.isArray(blocks) ? blocks.map(normalizeImportBlock) : []
}

// --- Draft flush callback (registered by importDraftDB) ---
let onFlushDraft: ((payload: DraftFlushPayload) => Promise<void>) | null = null

export interface DraftFlushPayload {
  kbId: string
  setId: string
  strategyPreset: string
  defaultDifficulty: string
  importMode: string
  importFormat: string
  currentStep: WorkbenchStep
  blockOrder: string[]
  blockMap: Record<string, ImportBlock>
  deletedBlockStack: string[]
  deletedBlockMap: Record<string, ImportBlock>
  questions: ImportQuestionItem[]
  dirtyBlockIds: string[]
}

export function registerDraftFlushHandler(fn: (payload: DraftFlushPayload) => Promise<void>) {
  onFlushDraft = fn
}

export const useImportWorkbenchStore = defineStore('importWorkbench', () => {
  // --- Core state ---
  const kbId = ref('')
  const setId = ref('')
  const strategyPreset = ref('general')
  const defaultDifficulty = ref('medium')
  const importMode = ref<'single' | 'batch'>('batch')
  const importFormat = ref<'json' | 'word' | 'pdf'>('word')
  const currentStep = ref<WorkbenchStep>('block-review')

  // P0: split blocks into order + map for O(1) access
  const blockOrder = ref<string[]>([])
  const blockMap = shallowRef<Record<string, ImportBlock>>({})
  const selectedBlockId = ref<string | null>(null)
  const anomalyFilter = ref<'all' | 'error' | 'warning'>('all')

  // Deleted blocks: stack for LIFO restore, map for content
  const deletedBlockStack = ref<string[]>([])
  const deletedBlockMap = shallowRef<Record<string, ImportBlock>>({})

  // P3: structural anomalies separated from block.anomalies
  const structuralAnomalies = shallowRef<Record<string, ImportBlock['anomalies']>>({})

  // Question review state
  const questions = ref<ImportQuestionItem[]>([])
  const questionErrors = ref<{ line_number: number; message: string }[]>([])
  const questionWarnings = ref<string[]>([])
  const questionStats = ref({ detected_questions: 0, with_answer: 0, without_answer: 0 })
  const isParsing = ref(false)
  const isImporting = ref(false)
  const draftExists = ref(false)

  // --- P2: Dirty tracking for incremental IndexedDB saves ---
  const dirtyBlockIds = new Set<string>()
  let metaDirty = false
  let flushTimer: ReturnType<typeof setTimeout> | null = null

  function markBlockDirty(id: string) {
    dirtyBlockIds.add(id)
    scheduleDraftFlush()
  }
  function markMetaDirty() {
    metaDirty = true
    scheduleDraftFlush()
  }

  function scheduleDraftFlush() {
    if (flushTimer) clearTimeout(flushTimer)
    flushTimer = setTimeout(() => {
      flushTimer = null
      void flushDraftChanges()
    }, 2000)
  }

  async function flushDraftChanges(): Promise<void> {
    if (flushTimer) { clearTimeout(flushTimer); flushTimer = null }
    if (!onFlushDraft) return
    const dirty = [...dirtyBlockIds]
    dirtyBlockIds.clear()
    metaDirty = false
    try {
      await onFlushDraft({
        kbId: kbId.value,
        setId: setId.value,
        strategyPreset: strategyPreset.value,
        defaultDifficulty: defaultDifficulty.value,
        importMode: importMode.value,
        importFormat: importFormat.value,
        currentStep: currentStep.value,
        blockOrder: blockOrder.value,
        blockMap: blockMap.value,
        deletedBlockStack: deletedBlockStack.value,
        deletedBlockMap: deletedBlockMap.value,
        questions: questions.value,
        dirtyBlockIds: dirty,
      })
    } catch { /* background save — ignore */ }
  }

  function ensureBlockMapMutable() {
    // shallowRef needs explicit trigger — replace the object to notify watchers
    blockMap.value = { ...blockMap.value }
  }

  function ensureDeletedBlockMapMutable() {
    deletedBlockMap.value = { ...deletedBlockMap.value }
  }

  // --- Computed ---

  const orderedBlocks = computed(() =>
    blockOrder.value.map(id => blockMap.value[id]).filter((b): b is ImportBlock => !!b)
  )

  const selectedBlock = computed(() => {
    if (!selectedBlockId.value) return null
    return blockMap.value[selectedBlockId.value] ?? null
  })

  const hasDeletedBlocks = computed(() => deletedBlockStack.value.length > 0)

  const filteredBlocks = computed(() => {
    const all = orderedBlocks.value
    if (anomalyFilter.value === 'all') return all
    return all.filter(b => {
      const blockAnoms = Array.isArray(b.anomalies) ? b.anomalies : []
      const structAnoms = structuralAnomalies.value[b.id] || []
      const allAnoms = [...blockAnoms, ...structAnoms]
      return allAnoms.some(a => a?.severity === anomalyFilter.value)
    })
  })

  const summary = computed(() => {
    let blocksWithAnomalies = 0
    let questionNumbers = 0
    const breakdown: Record<string, number> = {}
    for (const id of blockOrder.value) {
      const b = blockMap.value[id]
      if (!b) continue
      if (b.question_number != null) questionNumbers++
      const blockAnoms = Array.isArray(b.anomalies) ? b.anomalies : []
      const structAnoms = structuralAnomalies.value[id] || []
      const allAnoms = [...blockAnoms, ...structAnoms]
      if (allAnoms.length > 0) {
        blocksWithAnomalies++
        for (const a of allAnoms) {
          if (a && typeof a.code === 'string') {
            breakdown[a.code] = (breakdown[a.code] || 0) + 1
          }
        }
      }
    }
    return {
      total_blocks: blockOrder.value.length,
      blocks_with_anomalies: blocksWithAnomalies,
      question_numbers: questionNumbers,
      anomaly_breakdown: breakdown,
    }
  })

  function totalAnomaliesForBlock(blockId: string): ImportBlock['anomalies'] {
    const block = blockMap.value[blockId]
    const blockAnoms = block && Array.isArray(block.anomalies) ? block.anomalies : []
    const structAnoms = structuralAnomalies.value[blockId] || []
    return [...blockAnoms, ...structAnoms]
  }

  // --- Helpers ---

  function extractQuestionNumber(text: string): number | null {
    if (typeof text !== 'string') return null
    const m = text.match(/^\s*(\d+)[\.\)、]/)
    if (m) {
      const n = parseInt(m[1], 10)
      if (n > 0 && n <= 99999) return n
    }
    const m2 = text.match(/^\s*(\d{1,4})\s+[一-鿿]/)
    if (m2) {
      const n = parseInt(m2[1], 10)
      if (n > 0 && n <= 9999) return n
    }
    return null
  }

  function selectBlock(id: string | null) { selectedBlockId.value = id }

  function clearImportWarnings() {
    questionErrors.value = []
    questionWarnings.value = []
  }

  // --- Block operations (P0: O(1) access, no full-array iteration) ---

  function updateBlockText(id: string, text: string) {
    const block = blockMap.value[id]
    if (!block) return
    ensureBlockMapMutable()
    blockMap.value[id] = { ...block, current_text: text }
    markBlockDirty(id)
  }

  function restoreOriginalText(id: string) {
    const block = blockMap.value[id]
    if (!block) return
    ensureBlockMapMutable()
    blockMap.value[id] = {
      ...block,
      current_text: block.original_text,
      question_number: extractQuestionNumber(block.original_text),
    }
    markBlockDirty(id)
    scheduleValidateStructural()
  }

  function deleteBlock(id: string) {
    const block = blockMap.value[id]
    if (!block) return
    const idx = blockOrder.value.indexOf(id)
    if (idx < 0) return

    // Move to deleted
    ensureDeletedBlockMapMutable()
    deletedBlockStack.value.push(id)
    deletedBlockMap.value = { ...deletedBlockMap.value, [id]: block }

    // Remove from active
    blockOrder.value = blockOrder.value.filter(bid => bid !== id)
    ensureBlockMapMutable()
    const newMap = { ...blockMap.value }
    delete newMap[id]
    blockMap.value = newMap

    // Update selection
    if (selectedBlockId.value === id) {
      selectedBlockId.value = blockOrder.value.length > 0
        ? blockOrder.value[Math.min(idx, blockOrder.value.length - 1)]
        : null
    }

    markMetaDirty()
    scheduleValidateStructural()
  }

  function restoreBlock(id: string) {
    const block = deletedBlockMap.value[id]
    if (!block) return
    const stackIdx = deletedBlockStack.value.lastIndexOf(id)
    if (stackIdx < 0) return

    // Remove from deleted
    deletedBlockStack.value = deletedBlockStack.value.filter(bid => bid !== id)
    ensureDeletedBlockMapMutable()
    const newDeletedMap = { ...deletedBlockMap.value }
    delete newDeletedMap[id]
    deletedBlockMap.value = newDeletedMap

    // Insert back into active at the block's stored index position (clamped)
    const insertIdx = Math.min(block.index ?? blockOrder.value.length, blockOrder.value.length)
    const newOrder = [...blockOrder.value]
    newOrder.splice(insertIdx, 0, id)
    blockOrder.value = newOrder
    ensureBlockMapMutable()
    blockMap.value = { ...blockMap.value, [id]: block }

    markBlockDirty(id)
    markMetaDirty()
    scheduleValidateStructural()
  }

  function restoreAllDeleted() {
    if (deletedBlockStack.value.length === 0) return
    // Batch restore: build new state in one pass instead of O(n) shallowRef mutations
    const ids = [...deletedBlockStack.value].reverse() // restore oldest first for correct position
    const newMap = { ...blockMap.value }
    const newDeletedMap = { ...deletedBlockMap.value }
    const newOrder = [...blockOrder.value]
    for (const id of ids) {
      const block = deletedBlockMap.value[id]
      if (!block) continue
      const insertIdx = Math.min(block.index ?? newOrder.length, newOrder.length)
      newOrder.splice(insertIdx, 0, id)
      newMap[id] = block
      delete newDeletedMap[id]
      markBlockDirty(id)
    }
    blockOrder.value = newOrder
    blockMap.value = newMap
    deletedBlockMap.value = newDeletedMap
    deletedBlockStack.value = []
    markMetaDirty()
    scheduleValidateStructural()
  }

  function splitBlock(id: string, splitPositions: number[]) {
    const block = blockMap.value[id]
    if (!block || splitPositions.length === 0) return
    const text = block.current_text
    const parts: string[] = []
    let prev = 0
    for (const pos of splitPositions.sort((a, b) => a - b)) {
      if (pos > prev) parts.push(text.slice(prev, pos).trim())
      prev = pos
    }
    if (prev < text.length) parts.push(text.slice(prev).trim())
    if (parts.length <= 1) return

    const idx = blockOrder.value.indexOf(id)
    if (idx < 0) return

    const newIds: string[] = []
    const newEntries: Record<string, ImportBlock> = {}
    for (let i = 0; i < parts.length; i++) {
      const part = parts[i]
      const qNum = i === 0 ? block.question_number : extractQuestionNumber(part)
      const newId = crypto.randomUUID()
      newIds.push(newId)
      newEntries[newId] = {
        ...block,
        id: newId,
        current_text: part,
        original_text: part,
        question_number: qNum,
        tags: i === 0 ? [...normalizeTags(block.tags)] : [],
        metadata: { ...(block.metadata && typeof block.metadata === 'object' ? block.metadata : {}) },
        anomalies: [],
      }
      markBlockDirty(newId)
    }

    // Replace old id with new ids in order
    const newOrder = [...blockOrder.value]
    newOrder.splice(idx, 1, ...newIds)
    blockOrder.value = newOrder

    ensureBlockMapMutable()
    const newMap = { ...blockMap.value }
    delete newMap[id]
    Object.assign(newMap, newEntries)
    blockMap.value = newMap

    markMetaDirty()
    scheduleValidateStructural()
  }

  function mergeWithPrevious(id: string) {
    const idx = blockOrder.value.indexOf(id)
    if (idx <= 0) return
    const prevId = blockOrder.value[idx - 1]
    const prev = blockMap.value[prevId]
    const curr = blockMap.value[id]
    if (!prev || !curr) return

    ensureBlockMapMutable()
    const newPrev: ImportBlock = {
      ...prev,
      current_text: prev.current_text + '\n' + curr.current_text,
      original_text: prev.original_text + '\n' + curr.original_text,
      anomalies: [],
    }
    const newMap = { ...blockMap.value, [prevId]: newPrev }
    delete newMap[id]
    blockMap.value = newMap

    blockOrder.value = blockOrder.value.filter(bid => bid !== id)

    markBlockDirty(prevId)
    markMetaDirty()
    scheduleValidateStructural()
  }

  function mergeWithNext(id: string) {
    const idx = blockOrder.value.indexOf(id)
    if (idx < 0 || idx >= blockOrder.value.length - 1) return
    const nextId = blockOrder.value[idx + 1]
    const curr = blockMap.value[id]
    const next = blockMap.value[nextId]
    if (!curr || !next) return

    ensureBlockMapMutable()
    const newCurr: ImportBlock = {
      ...curr,
      current_text: curr.current_text + '\n' + next.current_text,
      original_text: curr.original_text + '\n' + next.original_text,
      anomalies: [],
    }
    const newMap = { ...blockMap.value, [id]: newCurr }
    delete newMap[nextId]
    blockMap.value = newMap

    blockOrder.value = blockOrder.value.filter(bid => bid !== nextId)

    markBlockDirty(id)
    markMetaDirty()
    scheduleValidateStructural()
  }

  function sortBlocksByQuestionNumber() {
    const withNum: { id: string; n: number }[] = []
    const withoutNum: string[] = []
    for (const id of blockOrder.value) {
      const b = blockMap.value[id]
      if (b?.question_number != null) {
        withNum.push({ id, n: b.question_number })
      } else {
        withoutNum.push(id)
      }
    }
    withNum.sort((a, b) => a.n - b.n)
    blockOrder.value = [...withNum.map(x => x.id), ...withoutNum]
    markMetaDirty()
    scheduleValidateStructural()
  }

  // --- Tag editing ---

  function addTagToBlock(blockId: string, tag: string) {
    const trimmed = tag.trim()
    if (!trimmed) return
    const block = blockMap.value[blockId]
    if (!block) return
    const tags = normalizeTags(block.tags)
    if (tags.includes(trimmed)) {
      MessagePlugin.warning('标签已存在')
      return
    }
    tags.push(trimmed)
    ensureBlockMapMutable()
    blockMap.value = { ...blockMap.value, [blockId]: { ...block, tags } }
    markBlockDirty(blockId)
  }

  function removeTagFromBlock(blockId: string, tag: string) {
    const block = blockMap.value[blockId]
    if (!block) return
    const tags = normalizeTags(block.tags).filter(t => t !== tag)
    ensureBlockMapMutable()
    blockMap.value = { ...blockMap.value, [blockId]: { ...block, tags } }
    markBlockDirty(blockId)
  }

  // --- P3: Structural validation ---

  let validateTimer: ReturnType<typeof setTimeout> | null = null

  function scheduleValidateStructural() {
    if (validateTimer) clearTimeout(validateTimer)
    validateTimer = setTimeout(() => {
      validateTimer = null
      validateStructuralAnomalies()
    }, 300)
  }

  function validateStructuralAnomalies() {
    if (validateTimer) { clearTimeout(validateTimer); validateTimer = null }
    const newAnomalies: Record<string, ImportBlock['anomalies']> = {}
    const seen = new Map<number, string>()
    let prevNum: number | null = null

    for (const id of blockOrder.value) {
      const block = blockMap.value[id]
      if (!block) continue
      const n = block.question_number
      const list: ImportBlock['anomalies'] = []

      if (n != null) {
        if (seen.has(n)) {
          list.push({ code: 'DUPLICATE_QUESTION_NUMBER', severity: 'error', message: `题号 ${n} 重复` })
        }
        seen.set(n, id)
        if (prevNum != null && n < prevNum) {
          list.push({ code: 'NON_MONOTONIC_QUESTION_NUMBER', severity: 'warning', message: `题号 ${n} < ${prevNum}` })
        }
        if (prevNum != null && n > prevNum + 1) {
          list.push({ code: 'QUESTION_NUMBER_GAP', severity: 'warning', message: `题号 ${prevNum} → ${n}` })
        }
        prevNum = n
      } else {
        list.push({ code: 'MISSING_QUESTION_NUMBER', severity: 'info', message: '该分块未识别到题号' })
      }

      if (list.length > 0) {
        newAnomalies[id] = list
      }
    }

    structuralAnomalies.value = newAnomalies
  }

  // --- Initial load ---

  function setBlocksFromResponse(input: ImportBlock[] | null | undefined) {
    const normalized = normalizeImportBlocks(input)
    const order: string[] = []
    const map: Record<string, ImportBlock> = {}
    for (const b of normalized) {
      order.push(b.id)
      map[b.id] = b
    }
    blockOrder.value = order
    blockMap.value = map
    deletedBlockStack.value = []
    deletedBlockMap.value = {}
    selectedBlockId.value = order.length > 0 ? order[0] : null
    structuralAnomalies.value = {}
    dirtyBlockIds.clear()
    metaDirty = true
    validateStructuralAnomalies()
  }

  /** Rebuild store state from a loaded draft (P6 compat) */
  function loadFromDraft(draft: {
    blocks?: ImportBlock[]
    blockOrder?: string[]
    blockMap?: Record<string, ImportBlock>
    deletedBlockStack?: string[]
    deletedBlockMap?: Record<string, ImportBlock>
    questions?: ImportQuestionItem[]
    strategyPreset?: string
    defaultDifficulty?: string
    importMode?: string
    importFormat?: string
    currentStep?: WorkbenchStep
  }) {
    // P6: migrate old format
    if (Array.isArray(draft.blocks) && draft.blocks.length > 0) {
      const normalized = normalizeImportBlocks(draft.blocks)
      const order: string[] = []
      const map: Record<string, ImportBlock> = {}
      for (const b of normalized) {
        order.push(b.id)
        map[b.id] = b
      }
      blockOrder.value = order
      blockMap.value = map
      deletedBlockStack.value = []
      deletedBlockMap.value = {}
    } else {
      blockOrder.value = Array.isArray(draft.blockOrder) ? draft.blockOrder : []
      blockMap.value = (draft.blockMap && typeof draft.blockMap === 'object') ? draft.blockMap as Record<string, ImportBlock> : {}
      deletedBlockStack.value = Array.isArray(draft.deletedBlockStack) ? draft.deletedBlockStack : []
      deletedBlockMap.value = (draft.deletedBlockMap && typeof draft.deletedBlockMap === 'object') ? draft.deletedBlockMap as Record<string, ImportBlock> : {}
    }

    selectedBlockId.value = blockOrder.value.length > 0 ? blockOrder.value[0] : null
    structuralAnomalies.value = {}
    dirtyBlockIds.clear()
    metaDirty = true

    questions.value = Array.isArray(draft.questions) ? draft.questions : []
    if (draft.strategyPreset) strategyPreset.value = draft.strategyPreset
    if (draft.defaultDifficulty) defaultDifficulty.value = draft.defaultDifficulty
    if (draft.importMode) importMode.value = draft.importMode as 'single' | 'batch'
    if (draft.importFormat) importFormat.value = draft.importFormat as 'json' | 'word' | 'pdf'
    if (draft.currentStep) currentStep.value = draft.currentStep
    draftExists.value = true

    validateStructuralAnomalies()
  }

  // --- Parse questions ---

  async function parseQuestionsAction() {
    if (blockOrder.value.length === 0) {
      MessagePlugin.warning('请先完成 block review')
      return false
    }
    // Flush pending draft changes before parsing
    await flushDraftChanges()

    isParsing.value = true
    try {
      const ordered = orderedBlocks.value
      const result = await parseImportedBlocks(kbId.value, setId.value, {
        blocks: ordered,
        default_difficulty: defaultDifficulty.value,
        strategy_preset: strategyPreset.value,
      })
      questions.value = result.items ?? []
      questionErrors.value = result.errors ?? []
      questionWarnings.value = result.warnings ?? []
      questionStats.value = result.stats ?? { detected_questions: questions.value.length, with_answer: 0, without_answer: 0 }
      if (questions.value.length === 0) {
        MessagePlugin.warning('未解析到题目，请检查 block 内容')
        return false
      }
      return true
    } catch (e: any) {
      MessagePlugin.error(e?.message || '解析失败')
      return false
    } finally {
      isParsing.value = false
    }
  }

  function goToStep(step: WorkbenchStep) { currentStep.value = step }

  function reset() {
    blockOrder.value = []
    blockMap.value = {}
    selectedBlockId.value = null
    deletedBlockStack.value = []
    deletedBlockMap.value = {}
    structuralAnomalies.value = {}
    questions.value = []
    questionErrors.value = []
    questionWarnings.value = []
    questionStats.value = { detected_questions: 0, with_answer: 0, without_answer: 0 }
    currentStep.value = 'block-review'
    isParsing.value = false
    isImporting.value = false
    draftExists.value = false
    dirtyBlockIds.clear()
    metaDirty = false
    if (flushTimer) { clearTimeout(flushTimer); flushTimer = null }
  }

  return {
    // state
    kbId, setId, strategyPreset, defaultDifficulty, importMode, importFormat,
    currentStep,
    blockOrder, blockMap, selectedBlockId, anomalyFilter,
    deletedBlockStack, deletedBlockMap,
    structuralAnomalies,
    questions, questionErrors, questionWarnings, questionStats,
    clearImportWarnings,
    isParsing, isImporting, draftExists,
    // computed
    orderedBlocks, selectedBlock, hasDeletedBlocks, filteredBlocks, summary,
    // helpers
    totalAnomaliesForBlock, extractQuestionNumber,
    // block operations
    selectBlock, updateBlockText, restoreOriginalText,
    deleteBlock, restoreBlock, restoreAllDeleted,
    splitBlock, mergeWithPrevious, mergeWithNext,
    sortBlocksByQuestionNumber,
    addTagToBlock, removeTagFromBlock,
    // structural validation
    scheduleValidateStructural, validateStructuralAnomalies,
    // lifecycle
    setBlocksFromResponse, loadFromDraft,
    parseQuestionsAction, goToStep, reset,
    // draft flush
    flushDraftChanges, markBlockDirty, markMetaDirty,
  }
})
