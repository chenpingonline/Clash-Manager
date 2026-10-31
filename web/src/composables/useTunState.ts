import { onMounted, onUnmounted, readonly, ref } from 'vue'
import { api } from '@/services/api'
import { openStatusStream } from '@/services/status-stream'
import type { TunState } from '@/types/api'

const state = ref<TunState>({ phase: 'checking', message: '正在检测 TUN 状态…' })
let subscribers = 0
let stop: (() => void) | null = null
let revision = 0
function accept(next: TunState) { ++revision; state.value = next }
function uncertain() {
  accept({ ...state.value, phase: 'unknown', message: '暂时无法读取 TUN 状态，已保留原有设置', runtimeEnabled: undefined, interface: state.value.interface ? { ...state.value.interface, checked: false } : undefined })
}
async function refresh() {
  const expected = revision
  try {
    const next = await api<TunState>('/api/network/tun/state')
    if (expected === revision) accept(next)
  } catch {
    if (expected === revision) uncertain()
  }
}
export function useTunState() {
  onMounted(() => {
    if (++subscribers === 1) {
      void refresh()
      stop = openStatusStream<TunState>('/api/network/tun/state', accept, uncertain)
    }
  })
  onUnmounted(() => { if (--subscribers === 0) { stop?.(); stop = null; ++revision } })
  return { tunState: readonly(state), refreshTunState: refresh }
}
