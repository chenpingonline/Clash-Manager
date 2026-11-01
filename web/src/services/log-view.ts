import { ref } from 'vue'
import { api, errorMessage, isAbortError, jsonRequest } from './api'

export const logLineOptions = [100, 200, 500, 800, 2000]
export function validLogLimit(value: unknown): number {
  const number = Number(value)
  return logLineOptions.includes(number) ? number : 100
}
export const logDisplayLimit = ref(100)
export const logDisplayLoading = ref(false)
export const logDisplaySaving = ref(false)
export const logDisplayReady = ref(false)
export const logDisplayError = ref('')
let generation = 0
function responseLimit(value: { limit: number }) {
  if (!logLineOptions.includes(value.limit)) throw new Error('日志显示行数无效')
  return value.limit
}
export async function loadLogDisplayLimit(signal?: AbortSignal): Promise<void> {
  if (logDisplaySaving.value) return
  const request = ++generation
  logDisplayLoading.value = true
  logDisplayError.value = ''
  try {
    const value = responseLimit(await api<{ limit: number }>('/api/logs/display', { signal }))
    if (request === generation && !signal?.aborted) { logDisplayLimit.value = value; logDisplayReady.value = true }
  } catch (cause) {
    if (request === generation && !isAbortError(cause)) logDisplayError.value = errorMessage(cause)
    throw cause
  } finally { if (request === generation) logDisplayLoading.value = false }
}
export async function saveLogDisplayLimit(value: unknown): Promise<boolean> {
  if (logDisplaySaving.value || logDisplayLoading.value || !logDisplayReady.value) return false
  const request = ++generation
  logDisplaySaving.value = true
  logDisplayError.value = ''
  try {
    const saved = responseLimit(await api<{ limit: number }>('/api/logs/display', jsonRequest('PUT', { limit: validLogLimit(value) })))
    if (request === generation) logDisplayLimit.value = saved
    return true
  } catch (cause) { logDisplayError.value = errorMessage(cause); return false }
  finally { logDisplaySaving.value = false }
}
// Keep both retained and pending logs bounded during bursts or background tabs.
export function appendLogBatch<T>(current: T[], pending: T[], limit: number): T[] {
  return current.concat(pending).slice(-limit)
}
export function isLogBottom(scrollTop: number, clientHeight: number, scrollHeight: number): boolean {
  return scrollHeight - scrollTop - clientHeight <= 24
}
