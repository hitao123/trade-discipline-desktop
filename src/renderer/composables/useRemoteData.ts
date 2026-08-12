import { readonly, shallowRef } from 'vue'

import { APIError } from '@/renderer/lib/api'

export function useRemoteData<T>(loader: () => Promise<T>) {
  const data = shallowRef<T>()
  const loading = shallowRef(false)
  const error = shallowRef('')

  async function refresh() {
    loading.value = true
    error.value = ''
    try {
      data.value = await loader()
    }
    catch (cause) {
      error.value = cause instanceof APIError || cause instanceof Error ? cause.message : '加载失败'
    }
    finally {
      loading.value = false
    }
  }

  return { data: readonly(data), loading: readonly(loading), error: readonly(error), refresh }
}
