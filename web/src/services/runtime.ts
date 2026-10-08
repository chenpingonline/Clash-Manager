import { shallowRef } from 'vue'
import { api } from './api'

export type RuntimeInfo = {
  platform: 'fnos' | 'docker'
  version: string
  capabilities: { appIcons: boolean; appUpdates: boolean; hostProxyEnvironment: boolean; nativeFolderAuthorization: boolean; externalCore: boolean }
}
export const runtime = shallowRef<RuntimeInfo>({ platform: 'fnos', version: '', capabilities: { appIcons: true, appUpdates: true, hostProxyEnvironment: true, nativeFolderAuthorization: true, externalCore: true } })
export async function loadRuntime() { runtime.value = await api<RuntimeInfo>('/api/runtime') }
