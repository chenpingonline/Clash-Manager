import { computed, ref, watch } from 'vue'
import english from '@/locales/en-US.json'
import parameterMessages from '@/locales/message-params.json'

export type LanguagePreference = 'system' | 'zh-CN' | 'en-US'
const STORAGE_KEY = 'clash-manager.language.v1'
const messages: Record<string, string> = Object.assign(Object.create(null), english)
function browserLanguage(): 'zh-CN' | 'en-US' {
  return typeof navigator === 'undefined' || (navigator.language || '').toLowerCase().startsWith('zh') ? 'zh-CN' : 'en-US'
}
function savedPreference(): LanguagePreference {
  try {
    const value = localStorage.getItem(STORAGE_KEY)
    return value === 'zh-CN' || value === 'en-US' ? value : 'system'
  } catch { return 'system' }
}
export const languagePreference = ref<LanguagePreference>(savedPreference())
const detectedLanguage = ref(browserLanguage())
export const locale = computed(() => languagePreference.value === 'system' ? detectedLanguage.value : languagePreference.value)
export const getLocale = () => locale.value
export function setLanguage(value: LanguagePreference) {
  languagePreference.value = value === 'zh-CN' || value === 'en-US' ? value : 'system'
  try { localStorage.setItem(STORAGE_KEY, languagePreference.value) } catch { /* Storage may be disabled. */ }
}
const translatedSlots: Record<string, string[]> = parameterMessages
const escape = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
// Short label templates require explicit parameters; do not reinterpret arbitrary names or diagnostics.
const explicitOnly = new Set(['{arg0}名称', '{arg0}来源', '{arg0}候选', '{arg0}说明', '全局{arg0}', '{arg0} 条'])
// Parameterized application messages preserve names, addresses and diagnostic details.
const patterns = Object.entries(messages).filter(([key]) => /\{arg\d+\}/.test(key) && !explicitOnly.has(key))
  .sort(([a], [b]) => b.replace(/\{arg\d+\}/g, '').length - a.replace(/\{arg\d+\}/g, '').length)
  .map(([key, value]) => {
  const parameters: string[] = []
  let cursor = 0, expression = '^'
  for (const match of key.matchAll(/\{arg\d+\}/g)) {
    expression += escape(key.slice(cursor, match.index)) + '([\\s\\S]*?)'
    parameters.push(match[0]); cursor = match.index + match[0].length
  }
  expression += escape(key.slice(cursor)) + '$'
  return { key, regex: new RegExp(expression), parameters, value }
})
const prefixes = Object.keys(messages).filter(key => /[：:]\s*$/.test(key) && !key.includes('{arg')).sort((a, b) => b.length - a.length)
export function t(message: unknown, params?: Record<string, unknown>): string {
  return translate(message, params, 0)
}
function translate(message: unknown, params: Record<string, unknown> | undefined, depth: number): string {
  const source = message == null ? '' : String(message)
  if (locale.value === 'zh-CN' || depth > 8) return interpolate(source, params)
  const direct = messages[source]
  if (direct !== undefined) return interpolate(direct, params)
  const trimmed = source.trim()
  if (trimmed !== source && messages[trimmed] !== undefined) {
    return source.slice(0, source.indexOf(trimmed)) + interpolate(messages[trimmed]!, params) + source.slice(source.indexOf(trimmed) + trimmed.length)
  }
  if (!params && /[\u3400-\u9fff]/.test(source)) {
    for (const pattern of patterns) {
      const match = pattern.regex.exec(source)
      if (!match) continue
      const values = Object.fromEntries(pattern.parameters.map((key, index) => [key.slice(1, -1), match[index + 1]]))
      for (const slot of translatedSlots[pattern.key] || []) {
        if (values[slot] !== undefined) values[slot] = translate(values[slot], undefined, depth + 1)
      }
      return interpolate(pattern.value, values)
    }
  }
  if (!params && /[\u3400-\u9fff]/.test(source)) for (const prefix of prefixes) {
    if (source.startsWith(prefix)) return messages[prefix] + translate(source.slice(prefix.length), undefined, depth + 1)
  }
  return source
}
const countMessages = {
  '条规则': ['1 条规则', '{arg0} 条规则'],
  '个节点': ['1 个节点', '{arg0} 个节点'],
  '个代理组': ['1 个代理组', '{arg0} 个代理组'],
  '个活动连接': ['1 个活动连接', '{arg0} 个活动连接'],
  '条': ['1 条', '{arg0} 条'],
} as const
export function countLabel(count: number, unit: keyof typeof countMessages): string {
  return t(countMessages[unit][count === 1 ? 0 : 1], { arg0: count })
}
export function modeLabel(mode: string): string {
  if (locale.value === 'en-US' && mode === 'rule') return 'Rule'
  const labels: Record<string, string> = { rule: '规则', global: '全局', direct: '直连' }
  return t(labels[mode] || mode)
}
function interpolate(message: string, params?: Record<string, unknown>): string {
  return params ? message.replace(/\{([^{}]+)\}/g, (token, key: string) => params[key] === undefined ? token : String(params[key])) : message
}
if (typeof window !== 'undefined') {
  window.addEventListener('languagechange', () => { detectedLanguage.value = browserLanguage() })
  window.addEventListener('storage', event => {
    if (event.key === STORAGE_KEY) languagePreference.value = savedPreference()
  })
}
watch(locale, value => {
  if (typeof document !== 'undefined') document.documentElement.lang = value
}, { immediate: true })
