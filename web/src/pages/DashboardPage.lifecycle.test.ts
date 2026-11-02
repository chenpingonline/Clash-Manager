import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, ref, ssrContextKey } from 'vue'
import DashboardPage from './DashboardPage.vue'

const mocks = vi.hoisted(() => ({ api: vi.fn(), notify: vi.fn() }))
vi.mock('@/services/api', async importOriginal => ({ ...await importOriginal<object>(), api: mocks.api }))
vi.mock('@/services/toast', () => ({ notify: mocks.notify }))
vi.mock('@/services/i18n', () => ({ t: (value: unknown) => value }))
vi.mock('@/services/runtime', () => ({ runtime: { value: { capabilities: { hostProxyEnvironment: false } } } }))
vi.mock('@/composables/useCoreHealth', () => ({ refreshCoreHealth: async () => ({ online: true, configs: {} }), updateCoreBootstrap: vi.fn() }))
vi.mock('@/composables/useDelayTests', () => ({ useDelayTests: () => ({ testing: ref(false), get: vi.fn(), restore: async () => {}, setContext: vi.fn() }) }))
vi.mock('@/services/status-stream', () => ({ openStatusStream: () => () => {} }))

// Run the real page setup and Vue mount/unmount hooks without a browser DOM.
const renderer = createRenderer<Record<string, unknown>, Record<string, unknown>>({
  patchProp() {}, insert() {}, remove() {}, createElement: () => ({}),
  createText: () => ({}), createComment: () => ({}), setText() {}, setElementText() {},
  parentNode: () => null, nextSibling: () => null,
})
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const proxies = { proxies: { Manual: { type: 'Selector', all: ['Node-A'], now: 'Node-A' }, 'Node-A': { type: 'ss' } } }
async function flush() { for (let i = 0; i < 25; i++) await Promise.resolve() }
let app: ReturnType<typeof renderer.createApp> | undefined
let read: ReturnType<typeof deferred<typeof proxies>>
let write: ReturnType<typeof deferred<object>>
let signal: AbortSignal | undefined
function mount() {
  app = renderer.createApp({ ...DashboardPage, render: () => null })
  app.provide(ssrContextKey, { modules: new Set() })
  app.mount({})
}
beforeEach(() => {
  vi.useFakeTimers()
  vi.stubGlobal('window', { localStorage: { getItem: () => null, setItem() {} }, setTimeout, clearTimeout })
  vi.stubGlobal('document', { addEventListener() {}, removeEventListener() {} })
  vi.stubGlobal('EventSource', class { close() {} })
  mocks.notify.mockReset()
  read = deferred(); write = deferred(); signal = undefined
  let reads = 0
  mocks.api.mockReset().mockImplementation((path: string, options?: RequestInit) => {
    if (options?.method === 'PUT') { signal = options.signal as AbortSignal; return write.promise }
    if (path === '/api/proxies') { signal = options?.signal as AbortSignal; return ++reads === 1 ? Promise.resolve(proxies) : read.promise }
    if (path === '/api/profiles') return Promise.resolve({ items: [] })
    return Promise.resolve({})
  })
})
afterEach(() => { app?.unmount(); app = undefined; vi.unstubAllGlobals(); vi.useRealTimers() })

describe('dashboard node request lifecycle', () => {
  it('cancels pending writes and ignores their late completion after leaving', async () => {
    mount(); await flush()
    expect(signal?.aborted).toBe(false)
    app!.unmount(); app = undefined
    expect(signal?.aborted).toBe(true)
    write.resolve({}); await flush()
    expect(mocks.api.mock.calls.filter(([path]) => path === '/api/proxies')).toHaveLength(1)
    expect(mocks.notify).not.toHaveBeenCalled()
  })
  it.each(['resolve', 'reject'] as const)('ignores a late read %s after leaving', async outcome => {
    mount(); await flush(); write.resolve({}); await flush()
    expect(mocks.api.mock.calls.filter(([path]) => path === '/api/proxies')).toHaveLength(2)
    app!.unmount(); app = undefined
    expect(signal?.aborted).toBe(true)
    if (outcome === 'resolve') read.resolve(proxies)
    else read.reject(new Error('Controller unavailable'))
    await flush()
    expect(mocks.notify).not.toHaveBeenCalled()
  })
  it('reports an active read failure without claiming the node application failed', async () => {
    mount(); await flush(); write.resolve({}); await flush()
    read.reject(new Error('Controller unavailable')); await flush()
    expect(mocks.notify).toHaveBeenCalledWith('节点已应用，但读取节点状态失败，请刷新后重试', true)
  })
  it('still reports an actual write failure', async () => {
    mount(); await flush(); write.reject(new Error('Controller rejected selection')); await flush()
    expect(mocks.notify).toHaveBeenCalledWith('Controller rejected selection', true)
  })
  it('ignores a rejected write after leaving', async () => {
    mount(); await flush(); app!.unmount(); app = undefined
    write.reject(new Error('Controller unavailable')); await flush()
    expect(mocks.notify).not.toHaveBeenCalled()
  })
})
