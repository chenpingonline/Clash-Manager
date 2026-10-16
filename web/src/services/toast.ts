import { readonly, ref } from 'vue'
import { describeConfigError } from './config-error'

export interface ToastMessage {
  id: number
  text: string
  bad: boolean
  configError: boolean
}

const items = ref<ToastMessage[]>([])
let sequence = 0

// Failures need time to be read and can now be dismissed by hand; successes are transient.
const LIFETIME_MS = { info: 3200, error: 8000 }

export function notify(text: string, bad = false): void {
  const id = ++sequence
  const configError = bad && describeConfigError(text) !== null
  if (configError) items.value = items.value.filter(item => !item.configError)
  items.value.push({ id, text, bad, configError })
  if (configError) return
  window.setTimeout(() => {
    items.value = items.value.filter(item => item.id !== id)
  }, bad ? LIFETIME_MS.error : LIFETIME_MS.info)
}

export function dismissToast(id: number): void {
  items.value = items.value.filter(item => item.id !== id)
}

export function dismissConfigErrors(): void {
  items.value = items.value.filter(item => !item.configError)
}

export const toasts = readonly(items)
