import { computed, ref } from 'vue'
import { api } from '@/services/api'
import type { AppUpdateInfo, ManagerSettings } from '@/types/api'

const enabled = ref(true)
const info = ref<AppUpdateInfo>({})
const available = computed(() => enabled.value && info.value.sourceConfigured === true && info.value.updateAvailable === true)
let pending: Promise<AppUpdateInfo> | null = null

function applyResult(result: AppUpdateInfo) {
  const changedVersion = result.currentVersion && info.value.currentVersion && result.currentVersion !== info.value.currentVersion
  if (changedVersion || result.sourceConfigured === false || typeof result.updateAvailable === 'boolean') info.value = result
  else info.value = { ...result, ...info.value, currentVersion: result.currentVersion || info.value.currentVersion, platform: result.platform || info.value.platform }
}

async function check(signal?: AbortSignal) {
  if (pending) return pending
  pending = api<AppUpdateInfo>('/api/app/check-update', { method: 'POST', signal }).then(result => {
    applyResult(result)
    return result
  }).finally(() => { pending = null })
  return pending
}

async function initialize(signal?: AbortSignal) {
  const settings = await api<ManagerSettings>('/api/settings')
  enabled.value = settings.notifyAppUpdates !== false
  if (!enabled.value) return null
  return check(signal)
}

function setEnabled(value: boolean) {
  enabled.value = value
}

export function useAppUpdateNotice() {
  return { available, enabled, info, applyResult, check, initialize, setEnabled }
}
