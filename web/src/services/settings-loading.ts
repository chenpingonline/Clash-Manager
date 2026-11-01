import { reactive } from 'vue'
import { errorMessage } from './api'
export type SettingsSection = 'core' | 'network' | 'dns' | 'tun' | 'advanced' | 'behavior' | 'bypass' | 'update'
export type SettingsResource = 'system' | 'manager' | 'network' | 'environment' | 'update' | 'icons' | 'geo' | 'logDisplay'
export function settingsResources(section: SettingsSection, tab: string): SettingsResource[] {
  switch (section) {
    case 'core': return tab === 'geo' ? ['geo'] : tab === 'delay' ? ['manager'] : ['system', 'manager']
    case 'network': case 'dns': case 'tun': return ['network']
    case 'advanced': return ['system', 'network', 'environment']
    case 'behavior': return ['icons', 'logDisplay']
    case 'bypass': return ['manager']
    case 'update': return ['manager', 'update']
  }
}
// Page-local reads share in-flight work; failed reads remain retryable.
export function createSettingsLoader(loaders: Record<SettingsResource, (signal: AbortSignal) => Promise<void>>) {
  const state = reactive(Object.fromEntries(Object.keys(loaders).map(key => [key, { ready: false, pending: false, error: '' }])) as Record<SettingsResource, { ready: boolean; pending: boolean; error: string }>)
  const pending = new Map<SettingsResource, Promise<void>>()
  const controllers = new Set<AbortController>()
  let disposed = false
  function load(key: SettingsResource, force = false): Promise<void> {
    if (disposed) return Promise.resolve()
    const existing = pending.get(key)
    if (existing) return existing
    if (!force && state[key].ready) return Promise.resolve()
    const controller = new AbortController()
    controllers.add(controller)
    state[key].pending = true; state[key].error = ''
    const timer = setTimeout(() => controller.abort(), 20000)
    const task = Promise.resolve().then(() => loaders[key](controller.signal)).then(() => {
      if (!disposed) state[key].ready = true
    }).catch(cause => {
      if (!disposed) state[key].error = errorMessage(cause)
    }).finally(() => {
      clearTimeout(timer); controllers.delete(controller); pending.delete(key)
      if (!disposed) state[key].pending = false
    })
    pending.set(key, task)
    return task
  }
  return { state, load: (keys: SettingsResource[], force = false) => Promise.all(keys.map(key => load(key, force))), dispose: () => { disposed = true; controllers.forEach(controller => controller.abort()) } }
}
