import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import type { ImportBlock } from '@/api/question_block'
import type { ImportQuestionItem } from '@/api/question'
import { parseImportedBlocks } from '@/api/question_block'

export type WorkbenchStep = 'block-review' | 'question-review'

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

export const useImportWorkbenchStore = defineStore('importWorkbench', () => {
  const kbId = ref('')
  const setId = ref('')
  const strategyPreset = ref('general')
  const defaultDifficulty = ref('medium')
  const importMode = ref<'single' | 'batch'>('batch')
  const importFormat = ref<'json' | 'word' | 'pdf'>('word')
  const blocks = ref<ImportBlock[]>([])
  const currentStep = ref<WorkbenchStep>('block-review')
  const selectedBlockId = ref<string | null>(null)
  const anomalyFilter = ref<'all' | 'error' | 'warning'>('all')
  const deletedBlocks = ref<ImportBlock[]>([])
  const questions = ref<ImportQuestionItem[]>([])
  const questionErrors = ref<{ line_number: number; message: string }[]>([])
  const questionWarnings = ref<string[]>([])
  const questionStats = ref({ detected_questions: 0, with_answer: 0, without_answer: 0 })
  // Internal workbench overlay
  const loading = ref(false)
  const loadingText = ref('')
  const loadingLeaving = ref(false)

  // Global import overlay (highest z-index, covers all dialogs)
  const importLoading = ref(false)
  const importLoadingText = ref('')
  const importLoadingLeaving = ref(false)
  const isParsing = ref(false)
  const isImporting = ref(false)
  const draftExists = ref(false)

  async function withWorkbenchLoading(text: string, task: () => Promise<void>): Promise<void> {
    if (loading.value) return
    loading.value = true
    loadingText.value = text
    loadingLeaving.value = false
    try { await task() }
    finally {
      loadingLeaving.value = true; loading.value = false
      await new Promise(r => setTimeout(r, 500))
      loadingLeaving.value = false
    }
  }

  async function withImportLoading(text: string, task: () => Promise<void>): Promise<void> {
    if (importLoading.value) return
    importLoading.value = true
    importLoadingText.value = text
    importLoadingLeaving.value = false
    try { await task() }
    finally {
      importLoadingLeaving.value = true; importLoading.value = false
      await new Promise(r => setTimeout(r, 500))
      importLoadingLeaving.value = false
    }
  }

  /** Clear import-stage warnings/errors (call on import success) */
  function clearImportWarnings() {
    questionErrors.value = []
    questionWarnings.value = []
  }

  const summary = computed(() => {
    let blocksWithAnomalies = 0
    let questionNumbers = 0
    const breakdown: Record<string, number> = {}
    for (const b of blocks.value) {
      if (b.question_number != null) questionNumbers++
      const anomalies = Array.isArray(b.anomalies) ? b.anomalies : []
      if (anomalies.length > 0) {
        blocksWithAnomalies++
        for (const a of anomalies) {
          if (a && typeof a.code === 'string') {
            breakdown[a.code] = (breakdown[a.code] || 0) + 1
          }
        }
      }
    }
    return {
      total_blocks: blocks.value.length,
      blocks_with_anomalies: blocksWithAnomalies,
      question_numbers: questionNumbers,
      anomaly_breakdown: breakdown,
    }
  })

  const filteredBlocks = computed(() => {
    if (anomalyFilter.value === 'all') return blocks.value
    return blocks.value.filter(b => {
      const anomalies = Array.isArray(b.anomalies) ? b.anomalies : []
      return anomalies.some(a => a?.severity === anomalyFilter.value)
    })
  })

  const selectedBlock = computed(() => {
    if (!selectedBlockId.value) return null
    return blocks.value.find(b => b.id === selectedBlockId.value) ?? null
  })

  const hasDeletedBlocks = computed(() => deletedBlocks.value.length > 0)

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

  function updateBlockText(id: string, text: string) {
    const block = blocks.value.find(b => b.id === id)
    if (block) block.current_text = text
  }

  function restoreOriginalText(id: string) {
    const block = blocks.value.find(b => b.id === id)
    if (!block) return
    block.current_text = block.original_text
    block.anomalies = (Array.isArray(block.anomalies) ? block.anomalies : []).filter(a =>
      a && ['OPTION_ONLY_BLOCK', 'PAGE_NOISE_DETECTED', 'SECTION_HEADING_IN_STEM', 'QUESTION_TYPE_HEADING_IN_STEM'].includes(a.code)
    )
  }

  function deleteBlock(id: string) {
    const idx = blocks.value.findIndex(b => b.id === id)
    if (idx >= 0) {
      const [removed] = blocks.value.splice(idx, 1)
      deletedBlocks.value.push(removed)
      blocks.value.forEach((b, i) => { b.index = i })
      if (selectedBlockId.value === id) {
        selectedBlockId.value = blocks.value.length > 0 ? blocks.value[Math.min(idx, blocks.value.length - 1)].id : null
      }
      scheduleValidateBlocks()
    }
  }

  function restoreBlock(id: string) {
    const idx = deletedBlocks.value.findIndex(b => b.id === id)
    if (idx >= 0) {
      const [restored] = deletedBlocks.value.splice(idx, 1)
      blocks.value.splice(restored.index, 0, restored)
      blocks.value.forEach((b, i) => { b.index = i })
      scheduleValidateBlocks()
    }
  }

  function splitBlock(id: string, splitPositions: number[]) {
    const block = blocks.value.find(b => b.id === id)
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
    const idx = blocks.value.findIndex(b => b.id === id)
    if (idx < 0) return
    const newBlocks: ImportBlock[] = parts.map((part, i) => {
      const qNum = i === 0 ? block.question_number : extractQuestionNumber(part)
      return {
        ...block, id: crypto.randomUUID(), index: idx + i,
        current_text: part, original_text: part,
        question_number: qNum,
        tags: i === 0 ? [...normalizeTags(block.tags)] : [],
        metadata: { ...(block.metadata && typeof block.metadata === 'object' ? block.metadata : {}) },
        anomalies: [],
      }
    })
    blocks.value.splice(idx, 1, ...newBlocks)
    blocks.value.forEach((b, i) => { b.index = i })
    scheduleValidateBlocks()
  }

  function mergeWithPrevious(id: string) {
    const idx = blocks.value.findIndex(b => b.id === id)
    if (idx <= 0) return
    const prev = blocks.value[idx - 1]; const curr = blocks.value[idx]
    prev.current_text = prev.current_text + '\n' + curr.current_text
    prev.original_text = prev.original_text + '\n' + curr.original_text
    prev.anomalies = []
    blocks.value.splice(idx, 1)
    blocks.value.forEach((b, i) => { b.index = i })
    scheduleValidateBlocks()
  }

  function mergeWithNext(id: string) {
    const idx = blocks.value.findIndex(b => b.id === id)
    if (idx < 0 || idx >= blocks.value.length - 1) return
    const curr = blocks.value[idx]; const next = blocks.value[idx + 1]
    curr.current_text = curr.current_text + '\n' + next.current_text
    curr.original_text = curr.original_text + '\n' + next.original_text
    curr.anomalies = []
    blocks.value.splice(idx + 1, 1)
    blocks.value.forEach((b, i) => { b.index = i })
    scheduleValidateBlocks()
  }

  function sortBlocksByQuestionNumber() {
    const numbered = blocks.value.filter(b => b.question_number != null)
    const unnumbered = blocks.value.filter(b => b.question_number == null)
    numbered.sort((a, b) => (a.question_number ?? 0) - (b.question_number ?? 0))
    blocks.value = [...numbered, ...unnumbered]
    blocks.value.forEach((b, i) => { b.index = i })
  }

  let validateTimer: ReturnType<typeof setTimeout> | null = null

  function scheduleValidateBlocks() {
    if (validateTimer) clearTimeout(validateTimer)
    validateTimer = setTimeout(() => {
      validateTimer = null
      validateBlocks()
    }, 300)
  }

  function validateBlocks() {
    const seen = new Map<number, string>()
    let prevNum: number | null = null
    for (const block of blocks.value) {
      block.anomalies = (Array.isArray(block.anomalies) ? block.anomalies : []).filter(a =>
        a && ['OPTION_ONLY_BLOCK', 'PAGE_NOISE_DETECTED', 'SECTION_HEADING_IN_STEM', 'QUESTION_TYPE_HEADING_IN_STEM'].includes(a.code)
      )
      const n = block.question_number
      if (n != null) {
        if (seen.has(n)) block.anomalies.push({ code: 'DUPLICATE_QUESTION_NUMBER', severity: 'error', message: `题号 ${n} 重复` })
        seen.set(n, block.id)
        if (prevNum != null && n < prevNum) block.anomalies.push({ code: 'NON_MONOTONIC_QUESTION_NUMBER', severity: 'warning', message: `题号 ${n} < ${prevNum}` })
        if (prevNum != null && n > prevNum + 1) block.anomalies.push({ code: 'QUESTION_NUMBER_GAP', severity: 'warning', message: `题号 ${prevNum} → ${n}` })
        prevNum = n
      }
    }
  }

  function setBlocksFromResponse(input: ImportBlock[] | null | undefined) {
    const normalized = normalizeImportBlocks(input)
    blocks.value = normalized
    deletedBlocks.value = []
    selectedBlockId.value = normalized.length > 0 ? normalized[0].id : null
    validateBlocks()
  }

  // --- Tag editing (fix 5) ---
  function addTagToBlock(blockId: string, tag: string) {
    const trimmed = tag.trim()
    if (!trimmed) return
    const block = blocks.value.find(b => b.id === blockId)
    if (!block) return
    const tags = normalizeTags(block.tags)
    if (tags.includes(trimmed)) {
      MessagePlugin.warning('标签已存在')
      return
    }
    tags.push(trimmed)
    block.tags = tags
  }

  function removeTagFromBlock(blockId: string, tag: string) {
    const block = blocks.value.find(b => b.id === blockId)
    if (!block) return
    const tags = normalizeTags(block.tags).filter(t => t !== tag)
    block.tags = tags
  }

  // --- Parse questions action (fix 1) ---
  async function parseQuestionsAction() {
    if (blocks.value.length === 0) {
      MessagePlugin.warning('请先完成 block review')
      return false
    }
    isParsing.value = true
    try {
      const result = await parseImportedBlocks(kbId.value, setId.value, {
        blocks: blocks.value,
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
    blocks.value = []
    currentStep.value = 'block-review'
    selectedBlockId.value = null
    deletedBlocks.value = []
    questions.value = []
    questionErrors.value = []
    questionWarnings.value = []
    questionStats.value = { detected_questions: 0, with_answer: 0, without_answer: 0 }
    isParsing.value = false
    isImporting.value = false
    draftExists.value = false
  }

  return {
    kbId, setId, strategyPreset, defaultDifficulty, importMode, importFormat,
    blocks, summary, currentStep, selectedBlockId, anomalyFilter,
    deletedBlocks, questions, questionErrors, questionWarnings, questionStats,
    loading, loadingText, loadingLeaving, withWorkbenchLoading,
    importLoading, importLoadingText, importLoadingLeaving, withImportLoading, clearImportWarnings,
    isParsing, isImporting, draftExists,
    filteredBlocks, selectedBlock, hasDeletedBlocks,
    selectBlock, updateBlockText, restoreOriginalText, extractQuestionNumber,
    deleteBlock, restoreBlock, splitBlock, mergeWithPrevious, mergeWithNext,
    sortBlocksByQuestionNumber, validateBlocks,
    setBlocksFromResponse, addTagToBlock, removeTagFromBlock,
    parseQuestionsAction, goToStep, reset,
  }
})
