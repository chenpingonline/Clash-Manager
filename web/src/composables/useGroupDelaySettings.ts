import { reactive, type Ref } from 'vue'
import { api, errorMessage, jsonRequest } from '@/services/api'
import type { GroupDelayContext } from '@/types/api'

type Draft = { enabled: boolean; url: string; edited: boolean }

export function useGroupDelaySettings(context: Readonly<Ref<GroupDelayContext | null>>, setContext: (value: GroupDelayContext) => void) {
  const drafts = reactive(new Map<string, Draft>())
  const errors = reactive(new Map<string, string>())
  const saving = reactive(new Set<string>())
  const pending = new Map<string, Promise<boolean>>()
  // Context responses contain every group; serialize saves to keep them current.
  let queue = Promise.resolve(true)
  const keyFor = (group: string, scope = context.value?.scope) => JSON.stringify([scope, group])
  const isCustom = (group: string) => drafts.get(keyFor(group))?.enabled ?? context.value?.groups[group]?.mode === 'custom'
  const url = (group: string) => {
    const draft = drafts.get(keyFor(group))
    if (draft) return draft.url
    const target = context.value?.groups[group]
    return target?.mode === 'custom' ? target.url : ''
  }
  const defaultUrl = (group: string) => {
    const current = context.value
    const target = current?.groups[group]
    return (target?.mode === 'custom' ? target.configUrl : target?.url) || current?.globalUrl || ''
  }
  const error = (group: string) => errors.get(keyFor(group)) || ''
  const isSaving = (group: string) => saving.has(keyFor(group))

  function edit(group: string, value: string) {
    if (isSaving(group)) return
    drafts.set(keyFor(group), { enabled: true, url: value, edited: true })
    errors.delete(keyFor(group))
  }

  function save(group: string): Promise<boolean> {
    const scope = context.value?.scope
    if (!scope) return Promise.resolve(false)
    const key = keyFor(group, scope)
    const inFlight = pending.get(key)
    if (inFlight) return inFlight
    if (!drafts.has(key)) return Promise.resolve(true)
    saving.add(key)
    const task = queue.then(async () => {
      if (context.value?.scope !== scope) return false
      const draft = drafts.get(key)!
      // Opening an empty editor must not save the placeholder as an override,
      // or report a validation error when the user simply moves focus away.
      if (draft.enabled && !draft.edited && !draft.url && context.value.groups[group]?.mode !== 'custom') return true
      const mode = draft.enabled ? 'custom' : 'auto'
      const targetURL = draft.enabled ? draft.url.trim() : ''
      if (draft.enabled) {
        try {
          const parsed = new URL(targetURL)
          if (!['http:', 'https:'].includes(parsed.protocol) || !parsed.hostname || parsed.username || parsed.password || targetURL.length > 4096) throw new Error()
        } catch {
          errors.set(key, '请输入有效的 HTTP 或 HTTPS 测速网址')
          return false
        }
      }
      const target = context.value.groups[group]
      if (target?.mode === mode && (!draft.enabled || target.url === targetURL)) {
        drafts.delete(key)
        errors.delete(key)
        return true
      }
      try {
        const result = await api<GroupDelayContext>('/api/delay-settings', jsonRequest('PUT', { scope, group, mode, url: targetURL }))
        if (context.value?.scope !== scope || result.scope !== scope) return false
        setContext(result)
        drafts.delete(key)
        errors.delete(key)
        return true
      } catch (cause) {
        errors.set(key, errorMessage(cause))
        return false
      }
    }).finally(() => {
      saving.delete(key)
      pending.delete(key)
    })
    pending.set(key, task)
    queue = task
    return task
  }

  async function toggle(group: string) {
    if (!context.value || isSaving(group)) return
    const key = keyFor(group)
    const enabled = !isCustom(group)
    drafts.set(key, { enabled, url: enabled ? '' : url(group), edited: false })
    errors.delete(key)
    // Enabling reveals the editor. Disabling immediately restores defaults.
    if (!enabled && !await save(group)) drafts.delete(key)
  }

  return { isCustom, url, defaultUrl, error, isSaving, edit, save, toggle }
}
