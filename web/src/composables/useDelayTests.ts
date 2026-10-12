import { computed, reactive, readonly, ref } from 'vue'
import { createDelayTest, createGroupDelayTest, delayTargetKey, loadDelayTestStatus, openDelayTestStream, type DelayTestStatus } from '@/services/delay-tests'
import type { GroupDelayContext } from '@/types/api'

export type NodeDelayState = { value: number; state: 'idle' | 'testing' | 'done' | 'timeout' | 'error'; error?: string }

const idleStatus: DelayTestStatus = { state: 'idle', names: [], results: [] }
const status = ref<DelayTestStatus>(idleStatus)
const submitting = ref<DelayTestStatus | null>(null)
const context = ref<GroupDelayContext | null>(null)
const delays = reactive(new Map<string, NodeDelayState>())
const waiting = new Map<string, Set<(value: DelayTestStatus) => void>>()
let closeStream: (() => void) | null = null
let connectedJobId = ''
let loading: Promise<DelayTestStatus> | null = null

function settle(next: DelayTestStatus) {
  if (!next.jobId || next.state === 'running') return
  const listeners = waiting.get(next.jobId)
  waiting.delete(next.jobId)
  listeners?.forEach(resolve => resolve(next))
}

function applyStatus(next: DelayTestStatus) {
  status.value = next
  // Preserve each result's source/URL, even if completion arrives after a
  // subscription switch. Rendering selects only the current target context.
  next.results.forEach(result => delays.set(delayTargetKey(result), { value: result.delay || 0, state: result.state, ...(result.error ? { error: result.error } : {}) }))
  const targets = next.targets || next.names.map(name => ({ name }))
  if (next.state === 'running') {
    targets.forEach(target => delays.set(delayTargetKey(target), { value: 0, state: 'testing' }))
  } else if (next.state === 'done') {
    const completed = new Set(next.results.map(delayTargetKey))
    targets.forEach(target => {
      if (!completed.has(delayTargetKey(target))) delays.set(delayTargetKey(target), { value: 0, state: 'error' })
    })
  }
  settle(next)
}

function subscribe(next: DelayTestStatus) {
  if (next.state !== 'running' || !next.jobId || connectedJobId === next.jobId) return
  closeStream?.()
  connectedJobId = next.jobId
  closeStream = openDelayTestStream(next.jobId, update => {
    applyStatus(update)
    if (update.state !== 'running') {
      closeStream?.()
      closeStream = null
      connectedJobId = ''
    }
  })
}

export async function restoreDelayTest(): Promise<DelayTestStatus> {
  if (loading) return loading
  loading = loadDelayTestStatus().then(next => {
    applyStatus(next)
    subscribe(next)
    return next
  }).finally(() => { loading = null })
  return loading
}

function waitForCompletion(jobId: string): Promise<DelayTestStatus> {
  if (status.value.jobId === jobId && status.value.state !== 'running') return Promise.resolve(status.value)
  return new Promise(resolve => {
    const listeners = waiting.get(jobId) || new Set()
    listeners.add(resolve)
    waiting.set(jobId, listeners)
  })
}

async function followJob(next: DelayTestStatus): Promise<DelayTestStatus> {
  applyStatus(next)
  subscribe(next)
  return next.jobId ? waitForCompletion(next.jobId) : next
}
type PrepareDelayTest = () => Promise<boolean>

async function submitDelayTest(
  groups: Array<{ group?: string; names: string[] }>, groupBatch: boolean,
  create: (scope: string | undefined) => Promise<DelayTestStatus>, prepare?: PrepareDelayTest,
): Promise<DelayTestStatus | null> {
  if (submitting.value || status.value.state === 'running') throw new Error('已有测速任务正在进行')
  const current = context.value
  const targets = groups.flatMap(({ group, names }) => names.map(name => {
    const target = group ? current?.groups[group] : undefined
    return { name, group, scope: current?.scope, url: target?.url || current?.globalUrl, expected: target?.expected }
  }))
  // Show the submitted groups immediately, including while saving URL edits.
  // Keep cached node measurements intact until the server accepts the job.
  submitting.value = { state: 'running', names: targets.map(target => target.name), targets,
    groups: groups.flatMap(item => item.group ? [item.group] : []), groupBatch, results: [] }
  try {
    if (prepare && !await prepare()) return null
    if (current?.scope !== context.value?.scope) throw new Error('当前配置已切换，请刷新后重试')
    return followJob(await create(current?.scope))
  } finally {
    submitting.value = null
  }
}
export async function startDelayTest(names: string[], group?: string, prepare?: PrepareDelayTest): Promise<DelayTestStatus | null> {
  return submitDelayTest([{ group, names }], false, scope => createDelayTest(names, group, scope), prepare)
}
async function startGroups(groups: Array<{ group: string; names: string[] }>, prepare?: PrepareDelayTest): Promise<DelayTestStatus | null> {
  if (!context.value) throw new Error('当前配置已切换，请刷新后重试')
  return submitDelayTest(groups, true, scope => createGroupDelayTest(groups, scope!), prepare)
}
function getDelay(name: string, group: string): NodeDelayState | undefined {
  const current = context.value
  if (!current) return undefined
  const target = current.groups[group]
  return delays.get(delayTargetKey({ name, scope: current.scope, url: target?.url || current.globalUrl, expected: target?.expected }))
}

export function useDelayTests() {
  return {
    delays: readonly(delays),
    context: readonly(context),
    setContext: (value: GroupDelayContext) => { context.value = value },
    get: getDelay,
    startGroups,
    status: computed(() => submitting.value ? readonly(submitting.value) : readonly(status.value)),
    testing: computed(() => Boolean(submitting.value) || status.value.state === 'running'),
    restore: restoreDelayTest,
    start: startDelayTest,
  }
}
