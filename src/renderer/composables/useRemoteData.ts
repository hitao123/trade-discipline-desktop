import { readonly, shallowRef } from 'vue'

import { APIError } from '@/renderer/lib/api'

export function useRemoteData<T>(loader: (signal: AbortSignal) => Promise<T>) {
  const data = shallowRef<T>()
  const loading = shallowRef(false)
  const error = shallowRef('')
  let generation = 0
  let controller: AbortController | undefined

  async function refresh() {
    controller?.abort()
    controller = new AbortController()
    const current = ++generation
    const signal = controller.signal
    loading.value = true
    error.value = ''
    try {
      const result = await loader(signal)
      if (current !== generation)
        return
      data.value = result
    }
    catch (cause) {
      if (current !== generation || (cause instanceof DOMException && cause.name === 'AbortError'))
        return
      error.value = cause instanceof APIError || cause instanceof Error ? cause.message : '加载失败'
    }
    finally {
      if (current === generation)
        loading.value = false
    }
  }

  return { data: readonly(data), loading: readonly(loading), error: readonly(error), refresh }
}
