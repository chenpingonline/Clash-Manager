import { ref } from 'vue'
import { api } from '@/services/api'
import type { ManagerSettings } from '@/types/api'

const enabled = ref(true), ready = ref(false)
let revision = 0
function sync(settings: Pick<ManagerSettings, 'bypassPageEnabled'>) {
  revision++
  enabled.value = settings.bypassPageEnabled !== false
  ready.value = true
}
async function initialize(signal?: AbortSignal) {
  if (ready.value) return
  const started = revision
  const settings = await api<ManagerSettings>('/api/settings', { signal, cache: 'no-store' })
  // A newer settings save/read must win over an older startup response.
  if (started === revision) sync(settings)
}
export function useBypassPagePreference() { return { enabled, ready, sync, initialize } }
