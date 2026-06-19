import { defineStore } from 'pinia'
import { ref, nextTick } from 'vue'

export const useImportUIStore = defineStore('importUI', () => {
  const visible = ref(false)
  const blocking = ref(false)
  const leaving = ref(false)
  const loadingText = ref('')

  async function withImportLoading<T>(text: string, task: () => Promise<T> | T): Promise<T> {
    if (blocking.value) return await task()

    loadingText.value = text
    visible.value = true
    blocking.value = true
    leaving.value = false

    // Ensure the browser renders the overlay before heavy work
    await nextTick()
    await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))
    await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))

    try {
      return await task()
    } finally {
      blocking.value = false
      leaving.value = true

      window.setTimeout(() => {
        visible.value = false
        leaving.value = false
        loadingText.value = ''
      }, 500)
    }
  }

  return { visible, blocking, leaving, loadingText, withImportLoading }
})
