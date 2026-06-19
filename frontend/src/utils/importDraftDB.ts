import type { ImportBlock } from '@/api/question_block'

const DB_NAME = 'question-import-workbench'
const DB_VERSION = 1
const STORE_NAME = 'drafts'
const TTL_MS = 7 * 24 * 60 * 60 * 1000 // 7 days

export interface ImportDraft {
  kbId: string
  setId: string
  blocks: ImportBlock[]
  strategyPreset: string
  defaultDifficulty: string
  importMode: string
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

function draftKey(kbId: string, setId: string): string {
  return `${kbId}:${setId}`
}

export async function saveDraft(draft: ImportDraft): Promise<void> {
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    const key = draftKey(draft.kbId, draft.setId)
    store.put({ ...draft, timestamp: Date.now() }, key)
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
      if (!draft) {
        resolve(null)
        return
      }
      // Check TTL
      if (Date.now() - draft.timestamp > TTL_MS) {
        // Expired — delete and return null
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
    const key = draftKey(kbId, setId)
    store.delete(key)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

/** Clean all expired drafts. Call on app init and when entering workbench. */
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
