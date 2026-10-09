import { ref } from 'vue'
import { api, APP_PREFIX, errorMessage, jsonRequest } from '@/services/api'
import { loadRuntime } from '@/services/runtime'

const CONNECTION_TIMEOUT = 10_000

async function connect<T>(controller: AbortController, request: (signal: AbortSignal) => Promise<T>): Promise<T> {
  const timer = setTimeout(() => controller.abort(new Error('连接超时，请检查服务状态后重试')), CONNECTION_TIMEOUT)
  let onAbort: () => void = () => undefined
  const aborted = new Promise<never>((_, reject) => {
    onAbort = () => reject(controller.signal.reason)
    controller.signal.addEventListener('abort', onAbort, { once: true })
  })
  try { return await Promise.race([request(controller.signal), aborted]) }
  finally { clearTimeout(timer); controller.signal.removeEventListener('abort', onAbort) }
}

export function useStartupSession(nativeEntry = Boolean(APP_PREFIX)) {
  // fnOS is authenticated by its Gateway; render the native shell immediately.
  const ready = ref(nativeEntry), loading = ref(!nativeEntry), busy = ref(false), required = ref(false), error = ref('')
  const username = ref('admin'), password = ref('')
  let active: AbortController | null = null
  let disposed = false
  function begin() {
    active?.abort()
    const controller = new AbortController()
    active = controller
    return controller
  }
  const current = (controller: AbortController) => !disposed && active === controller

  async function initialize() {
    if (disposed) return
    const controller = begin()
    if (!nativeEntry) { loading.value = true; error.value = '' }
    try {
      if (nativeEntry) {
        await connect(controller, signal => loadRuntime(signal))
      } else {
        const session = await connect(controller, async signal => {
          const session = await api<{ required: boolean; authenticated: boolean }>('/api/auth/session', { signal })
          if (session.authenticated && !signal.aborted && current(controller)) await loadRuntime(signal)
          return session
        })
        if (current(controller)) { required.value = session.required; ready.value = session.authenticated }
      }
    } catch (cause) {
      if (!current(controller)) return
      if (nativeEntry) console.warn('Unable to load runtime information', cause)
      else error.value = errorMessage(cause)
    } finally {
      if (current(controller)) { loading.value = false; active = null }
    }
  }
  async function login() {
    if (nativeEntry || disposed || busy.value || loading.value) return
    const controller = begin()
    busy.value = true; error.value = ''
    try {
      await connect(controller, async signal => {
        await api('/api/auth/login', { ...jsonRequest('POST', { username: username.value, password: password.value }), signal })
        if (!current(controller) || signal.aborted) return
        password.value = ''
        await loadRuntime(signal)
      })
      if (current(controller)) ready.value = true
    } catch (cause) { if (current(controller)) error.value = errorMessage(cause) }
    finally { if (current(controller)) { busy.value = false; active = null } }
  }
  function expire(loggedOut = false) {
    if (nativeEntry || disposed) return
    const wasReady = ready.value
    const controller = active; active = null; controller?.abort()
    ready.value = false; required.value = true; loading.value = false; busy.value = false
    if (loggedOut) error.value = ''
    else if (wasReady) error.value = '登录已过期，请重新登录'
  }
  function dispose() {
    disposed = true
    const controller = active; active = null; controller?.abort()
  }
  return { ready, loading, busy, required, error, username, password, initialize, login, expire, dispose }
}
