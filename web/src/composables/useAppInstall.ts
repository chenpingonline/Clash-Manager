import { computed, ref } from 'vue'
import { api, errorMessage, jsonRequest } from '@/services/api'
import { useAppUpdateNotice } from '@/composables/useAppUpdateNotice'
import type { AppInstallResponse, AppInstallStatus } from '@/types/api'

const status = ref<AppInstallStatus>({ stage: 'idle', progress: 0 })
const supported = ref<boolean | null>(null)
const reason = ref('')
const disconnected = ref(false)
const submitting = ref(false)
let csrfToken = ''
const activeStages = ['queued', 'downloading', 'verifying', 'installing', 'restarting']
const active = computed(() => submitting.value || activeStages.includes(status.value.stage) || (status.value.stage === 'success' && disconnected.value))
let timer: ReturnType<typeof setTimeout> | null = null
let startedAt = 0

function apply(result: AppInstallResponse) {
  csrfToken = result.csrfToken || ''
  supported.value = result.supported
  reason.value = result.reason || ''
  status.value = result.status
  disconnected.value = false
}

function schedule() {
  if (timer) return
  timer = setTimeout(() => { timer = null; void poll() }, 2000)
}

async function poll() {
  if (!active.value) return
  try {
    apply(await api<AppInstallResponse>('/api/app/update/status', { signal: AbortSignal.timeout(5000) }))
    if (status.value.stage === 'success') {
      const { info, applyResult } = useAppUpdateNotice()
      const before = info.value.currentVersion
      const health = await api<{ ok: boolean; version: string }>('/api/health', { signal: AbortSignal.timeout(5000) })
      if (health.ok && health.version.replace(/^v/, '') === status.value.targetVersion?.replace(/^v/, '')) {
        applyResult({ ...info.value, currentVersion: health.version, updateAvailable: false, latest: undefined })
        if (before && before.replace(/^v/, '') !== health.version.replace(/^v/, '')) window.location.reload()
      } else throw new Error('Updated app version is not ready')
    }
  } catch {
    // The old gateway disappears while App Center installs the replacement.
    // Keep polling the same task instead of submitting another installation.
    disconnected.value = true
  }
  if (Date.now() - startedAt > 32 * 60 * 1000 && active.value) {
    status.value = { ...status.value, stage: 'error', error: '更新等待超时，请在飞牛应用中心检查状态' }
  }
  if (active.value) schedule()
}

async function refresh() {
  try {
    apply(await api<AppInstallResponse>('/api/app/update/status', { signal: AbortSignal.timeout(5000) }))
    if (active.value) { startedAt ||= Date.now(); schedule() }
  } catch (cause) { reason.value = errorMessage(cause) }
}

async function start(version: string) {
  if (active.value || supported.value !== true) return
  submitting.value = true
  disconnected.value = false
  startedAt = Date.now()
  try {
    // Read current status and the process nonce, without checking GitHub again.
    const current = await api<AppInstallResponse>('/api/app/update/status', { signal: AbortSignal.timeout(5000) })
    apply(current)
    if (!current.supported) return
    if (activeStages.includes(current.status.stage)) { schedule(); return }
    const request = jsonRequest('POST', { version })
    const headers = new Headers(request.headers)
    if (csrfToken) headers.set('X-Clash-App-Update-Token', csrfToken)
    apply(await api<AppInstallResponse>('/api/app/update', { ...request, headers, signal: AbortSignal.timeout(20000) }))
    schedule()
  } catch (cause) {
    // A lost response does not mean the independent system task did not start.
    // Reconcile its persisted state before allowing a retry.
    try {
      const result = await api<AppInstallResponse>('/api/app/update/status', { signal: AbortSignal.timeout(5000) })
      if (activeStages.includes(result.status.stage) && result.status.targetVersion?.replace(/^v/, '') === version.replace(/^v/, '')) { apply(result); schedule(); return }
    } catch { /* show the original request error */ }
    status.value = { stage: 'error', progress: 0, targetVersion: version, error: errorMessage(cause) }
  } finally { submitting.value = false }
}

export function useAppInstall() {
  return { status, supported, reason, disconnected, active, refresh, start }
}
