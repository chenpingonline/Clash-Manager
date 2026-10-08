import { computed, shallowRef } from 'vue'
import { api, APP_PREFIX } from './api'

export type RuntimeInfo = {
  platform: 'fnos' | 'docker'
  displayName?: string
  version: string
  capabilities: { appIcons: boolean; appUpdates: boolean; hostProxyEnvironment: boolean; nativeFolderAuthorization: boolean; externalCore: boolean }
}
export const runtime = shallowRef<RuntimeInfo>({ platform: APP_PREFIX ? 'fnos' : 'docker', version: '', capabilities: { appIcons: true, appUpdates: true, hostProxyEnvironment: true, nativeFolderAuthorization: true, externalCore: true } })
export const appDisplayName = computed(() => runtime.value.displayName || (runtime.value.platform === 'docker' ? 'Clash Manager' : 'Clash for fnOS'))
export async function loadRuntime() {
  runtime.value = await api<RuntimeInfo>('/api/runtime')
  document.title = appDisplayName.value
}
