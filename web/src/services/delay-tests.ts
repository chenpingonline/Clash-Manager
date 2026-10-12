import { api, APP_PREFIX, jsonRequest } from './api'

export type DelayTestTarget = {
  name: string
  scope?: string
  group?: string
  url?: string
  expected?: string
}
export function delayTargetKey(target: DelayTestTarget): string {
  if (!target.scope && !target.url) return target.name
  return JSON.stringify([target.scope || '', target.url || '', target.expected || '', target.name])
}
export type DelayTestResult = DelayTestTarget & {
  delay: number
  state: 'done' | 'timeout' | 'error'
  error?: string
}

export type DelayTestStatus = {
  jobId?: string
  state: 'idle' | 'running' | 'done'
  names: string[]
  targets?: DelayTestTarget[]
  groups?: string[]
  groupBatch?: boolean
  results: DelayTestResult[]
  createdAt?: number
  updatedAt?: number
}

type DelayTestProgress = {
  state: DelayTestStatus['state']
  targets?: readonly Readonly<DelayTestTarget>[]
  groups?: readonly string[]
  groupBatch?: boolean
}

export function delayTestGroupRunning(status: DelayTestProgress, scope: string | undefined, group: string): boolean {
  if (status.state !== 'running' || !scope || !status.targets?.some(target => target.scope === scope)) return false
  // Node measurements can be shared, but button state belongs to submitted groups.
  return status.groups?.includes(group) ?? status.targets.some(target => target.scope === scope && target.group === group)
}

export function delayTestGroupBatchRunning(status: DelayTestProgress, scope: string | undefined): boolean {
  return status.state === 'running' && status.groupBatch === true && Boolean(scope && status.targets?.some(target => target.scope === scope))
}

export function loadDelayTestStatus(): Promise<DelayTestStatus> {
  return api<DelayTestStatus>('/api/delays')
}

export function createDelayTest(names: string[], group?: string, scope?: string): Promise<DelayTestStatus> {
  return api<DelayTestStatus>('/api/delays', jsonRequest('POST', { names: [...new Set(names)], ...(group ? { group, scope } : {}) }))
}

export function openDelayTestStream(jobId: string, onUpdate: (status: DelayTestStatus) => void): () => void {
  const source = new EventSource(`${APP_PREFIX}/api/delays/${encodeURIComponent(jobId)}/stream`)
  source.onmessage = event => {
    try { onUpdate(JSON.parse(event.data) as DelayTestStatus) }
    catch { /* Ignore one malformed event; EventSource keeps the current task subscription alive. */ }
  }
  return () => source.close()
}

export function createGroupDelayTest(groups: Array<{ group: string; names: string[] }>, scope: string): Promise<DelayTestStatus> {
  return api<DelayTestStatus>('/api/delays', jsonRequest('POST', { groups, scope }))
}
