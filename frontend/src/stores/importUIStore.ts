import { defineStore } from 'pinia'
import { ref, nextTick } from 'vue'

export const useImportUIStore = defineStore('importUI', () => {
  const loading = ref(false)
  const loadingText = ref('')
  const loadingLeaving = ref(false)

  async function withImportLoading<T>(text: string, task: () => Promise<T> | T): Promise<T> {
    if (loading.value) return await task()

    loadingText.value = text
    loadingLeaving.value = false
    loading.value = true

    // Ensure the browser renders the overlay before heavy work
    await nextTick()
    await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))

    try {
      return await task()
    } finally {
      loadingLeaving.value = true
      await new Promise(resolve => setTimeout(resolve, 500))
      loading.value = false
      loadingLeaving.value = false
      loadingText.value = ''
    }
  }

  return { loading, loadingText, loadingLeaving, withImportLoading }
})
