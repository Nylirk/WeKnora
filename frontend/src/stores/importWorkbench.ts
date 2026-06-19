import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { ImportBlock, ImportBlockAnomaly } from '@/api/question_block'
import type { ImportQuestionItem } from '@/api/question'

export type WorkbenchStep = 'block-review' | 'question-review'

export const useImportWorkbenchStore = defineStore('importWorkbench', () => {
  // --- Persistent state (saved to draft) ---
  const kbId = ref('')
  const setId = ref('')
  const strategyPreset = ref('general')
  const defaultDifficulty = ref('medium')
  const importMode = ref<'single' | 'batch'>('batch')
  const blocks = ref<ImportBlock[]>([])
  const summary = ref({ total_blocks: 0, blocks_with_anomalies: 0, question_numbers: 0, anomaly_breakdown: {} as Record<string, number> })

  // --- Transient state (not saved) ---
  const currentStep = ref<WorkbenchStep>('block-review')
  const selectedBlockId = ref<string | null>(null)
  const anomalyFilter = ref<'all' | 'error' | 'warning'>('all')
  const deletedBlocks = ref<ImportBlock[]>([])  // undo stack
  const questions = ref<ImportQuestionItem[]>([])
  const questionErrors = ref<{ line_number: number; message: string }[]>([])
  const questionWarnings = ref<string[]>([])
  const questionStats = ref({ detected_questions: 0, with_answer: 0, without_answer: 0 })
  const isParsing = ref(false)
  const isImporting = ref(false)
  const draftExists = ref(false)

  // --- Computed ---
  const filteredBlocks = computed(() => {
    if (anomalyFilter.value === 'all') return blocks.value
    return blocks.value.filter(b =>
      b.anomalies.some(a => a.severity === anomalyFilter.value)
    )
  })

  const selectedBlock = computed(() => {
    if (!selectedBlockId.value) return null
    return blocks.value.find(b => b.id === selectedBlockId.value) ?? null
  })

  const hasDeletedBlocks = computed(() => deletedBlocks.value.length > 0)

  // --- Block operations ---
  function selectBlock(id: string | null) {
    selectedBlockId.value = id
  }

  function updateBlockText(id: string, text: string) {
    const block = blocks.value.find(b => b.id === id)
    if (block) block.current_text = text
  }

  function deleteBlock(id: string) {
    const idx = blocks.value.findIndex(b => b.id === id)
    if (idx >= 0) {
      const [removed] = blocks.value.splice(idx, 1)
      deletedBlocks.value.push(removed)
      // Re-index
      blocks.value.forEach((b, i) => { b.index = i })
      if (selectedBlockId.value === id) {
        selectedBlockId.value = blocks.value.length > 0 ? blocks.value[Math.min(idx, blocks.value.length - 1)].id : null
      }
    }
  }

  function restoreBlock(id: string) {
    const idx = deletedBlocks.value.findIndex(b => b.id === id)
    if (idx >= 0) {
      const [restored] = deletedBlocks.value.splice(idx, 1)
      blocks.value.splice(restored.index, 0, restored)
      blocks.value.forEach((b, i) => { b.index = i })
    }
  }

  function splitBlock(id: string, splitPositions: number[]) {
    const block = blocks.value.find(b => b.id === id)
    if (!block || splitPositions.length === 0) return

    const text = block.current_text
    const parts: string[] = []
    let prev = 0
    for (const pos of splitPositions.sort((a, b) => a - b)) {
      if (pos > prev) {
        parts.push(text.slice(prev, pos).trim())
      }
      prev = pos
    }
    if (prev < text.length) {
      parts.push(text.slice(prev).trim())
    }
    if (parts.length <= 1) return

    const idx = blocks.value.findIndex(b => b.id === id)
    if (idx < 0) return

    const newBlocks: ImportBlock[] = parts.map((text2, i) => ({
      ...block,
      id: crypto.randomUUID(),
      index: idx + i,
      current_text: text2,
      original_text: text2,
      question_number: i === 0 ? block.question_number : null,
      tags: i === 0 ? [...block.tags] : [],
      metadata: { ...block.metadata },
      anomalies: [],
    }))

    blocks.value.splice(idx, 1, ...newBlocks)
    blocks.value.forEach((b, i) => { b.index = i })

    // Re-validate after split
    validateBlocks()
  }

  function mergeWithPrevious(id: string) {
    const idx = blocks.value.findIndex(b => b.id === id)
    if (idx <= 0) return
    const prev = blocks.value[idx - 1]
    const curr = blocks.value[idx]
    prev.current_text = prev.current_text + '\n' + curr.current_text
    prev.original_text = prev.original_text + '\n' + curr.original_text
    prev.anomalies = [] // clear stale anomalies, will be re-validated
    blocks.value.splice(idx, 1)
    blocks.value.forEach((b, i) => { b.index = i })
    validateBlocks()
  }

  function mergeWithNext(id: string) {
    const idx = blocks.value.findIndex(b => b.id === id)
    if (idx < 0 || idx >= blocks.value.length - 1) return
    const curr = blocks.value[idx]
    const next = blocks.value[idx + 1]
    curr.current_text = curr.current_text + '\n' + next.current_text
    curr.original_text = curr.original_text + '\n' + next.original_text
    curr.anomalies = []
    blocks.value.splice(idx + 1, 1)
    blocks.value.forEach((b, i) => { b.index = i })
    validateBlocks()
  }

  function sortBlocksByQuestionNumber() {
    const numbered = blocks.value.filter(b => b.question_number != null)
    const unnumbered = blocks.value.filter(b => b.question_number == null)
    numbered.sort((a, b) => (a.question_number ?? 0) - (b.question_number ?? 0))
    blocks.value = [...numbered, ...unnumbered]
    blocks.value.forEach((b, i) => { b.index = i })
  }

  function validateBlocks() {
    // Client-side re-validation: mark duplicate numbers etc.
    const seen = new Map<number, string>()
    let prevNum: number | null = null
    for (const block of blocks.value) {
      // Clear computed anomalies, keep only static ones (OPTION_ONLY_BLOCK etc.)
      block.anomalies = block.anomalies.filter(a =>
        ['OPTION_ONLY_BLOCK', 'PAGE_NOISE_DETECTED', 'SECTION_HEADING_IN_STEM', 'QUESTION_TYPE_HEADING_IN_STEM'].includes(a.code)
      )
      const n = block.question_number
      if (n != null) {
        if (seen.has(n)) {
          block.anomalies.push({ code: 'DUPLICATE_QUESTION_NUMBER', severity: 'error', message: `题号 ${n} 重复` })
        }
        seen.set(n, block.id)
        if (prevNum != null && n < prevNum) {
          block.anomalies.push({ code: 'NON_MONOTONIC_QUESTION_NUMBER', severity: 'warning', message: `题号 ${n} < ${prevNum}` })
        }
        if (prevNum != null && n > prevNum + 1) {
          block.anomalies.push({ code: 'QUESTION_NUMBER_GAP', severity: 'warning', message: `题号 ${prevNum} → ${n}` })
        }
        prevNum = n
      }
    }
  }

  function setBlocksFromResponse(b: ImportBlock[], s: typeof summary.value) {
    blocks.value = b
    summary.value = s
    deletedBlocks.value = []
    selectedBlockId.value = b.length > 0 ? b[0].id : null
  }

  // --- Step navigation ---
  function goToStep(step: WorkbenchStep) {
    currentStep.value = step
  }

  // --- Reset ---
  function reset() {
    blocks.value = []
    summary.value = { total_blocks: 0, blocks_with_anomalies: 0, question_numbers: 0, anomaly_breakdown: {} }
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
    kbId, setId, strategyPreset, defaultDifficulty, importMode,
    blocks, summary, currentStep, selectedBlockId, anomalyFilter,
    deletedBlocks, questions, questionErrors, questionWarnings, questionStats,
    isParsing, isImporting, draftExists,
    filteredBlocks, selectedBlock, hasDeletedBlocks,
    selectBlock, updateBlockText, deleteBlock, restoreBlock,
    splitBlock, mergeWithPrevious, mergeWithNext,
    sortBlocksByQuestionNumber, validateBlocks,
    setBlocksFromResponse, goToStep, reset,
  }
})
