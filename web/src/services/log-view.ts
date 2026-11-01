import { ref } from 'vue'

export const logLineOptions = [100, 200, 500, 800, 2000]
const storageKey = 'clash.logDisplayLimit'
export function validLogLimit(value: unknown): number {
  const number = Number(value)
  return logLineOptions.includes(number) ? number : 100
}
function readLimit(): number {
  try { return validLogLimit(localStorage.getItem(storageKey)) } catch { return 100 }
}
export const logDisplayLimit = ref(readLimit())
export function saveLogDisplayLimit(value: unknown) {
  logDisplayLimit.value = validLogLimit(value)
  try { localStorage.setItem(storageKey, String(logDisplayLimit.value)) } catch { /* Session preference still works. */ }
}
// Keep both retained and pending logs bounded during bursts or background tabs.
export function appendLogBatch<T>(current: T[], pending: T[], limit: number): T[] {
  return current.concat(pending).slice(-limit)
}
export function isLogBottom(scrollTop: number, clientHeight: number, scrollHeight: number): boolean {
  return scrollHeight - scrollTop - clientHeight <= 24
}
