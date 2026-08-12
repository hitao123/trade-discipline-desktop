import type { ApiEnvelope } from '@/shared/contracts'

export class APIError extends Error {
  readonly code: string
  readonly fields: Record<string, string>

  constructor(code: string, message: string, fields: Record<string, string> = {}) {
    super(message)
    this.name = 'APIError'
    this.code = code
    this.fields = fields
  }
}

function backendConfig() {
  const bridge = window.discipline
  return {
    baseURL: bridge?.apiBaseURL ?? import.meta.env.VITE_API_BASE_URL ?? '',
    token: bridge?.sessionToken ?? import.meta.env.VITE_SESSION_TOKEN ?? '',
  }
}

export const api = {
  async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const { baseURL, token } = backendConfig()
    const headers = new Headers(init.headers)
    headers.set('Accept', 'application/json')
    if (init.body) headers.set('Content-Type', 'application/json')
    if (token) headers.set('Authorization', `Bearer ${token}`)
    let response: Response
    try {
      response = await fetch(`${baseURL}${path}`, { ...init, headers })
    }
    catch (error) {
      throw new APIError('BACKEND_OFFLINE', `本地后端暂时不可用：${error instanceof Error ? error.message : '未知错误'}`)
    }
    const envelope = await response.json() as ApiEnvelope<T>
    if (!response.ok || !envelope.ok) {
      const failure = envelope.ok ? undefined : envelope.error
      throw new APIError(failure?.code ?? 'REQUEST_FAILED', failure?.message ?? `请求失败（${response.status}）`, failure?.fields)
    }
    return envelope.data
  },
}
