import { api, jsonRequest } from './api'

export async function bypassRequest<T>(action: 'preview' | 'configure' | 'stop' | 'preferences', body: unknown, signal?: AbortSignal): Promise<T> {
  // Fetch afresh so a gateway restart cannot leave a cached nonce in this page.
  const { csrfToken } = await api<{ csrfToken: string }>('/api/network/bypass/csrf', { cache: 'no-store', ...(signal ? { signal } : {}) })
  if (!csrfToken) throw new Error('获取接管请求令牌失败，请重新打开页面')
  const request = jsonRequest('POST', body)
  if (signal) request.signal = signal
  request.headers = { ...request.headers, 'X-Clash-Bypass-Token': csrfToken }
  return api<T>(`/api/network/bypass/${action}`, request)
}
