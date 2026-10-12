import { afterEach, describe, expect, it, vi } from 'vitest'
import { createDelayTest, delayTestGroupBatchRunning, delayTestGroupRunning, loadDelayTestStatus, openDelayTestStream, type DelayTestStatus } from './delay-tests'

class EventSourceMock {
  static instance: EventSourceMock | null = null
  onmessage: ((event: MessageEvent<string>) => void) | null = null
  closed = false
  constructor(readonly url: string) { EventSourceMock.instance = this }
  close() { this.closed = true }
}

afterEach(() => {
  EventSourceMock.instance = null
  vi.unstubAllGlobals()
})

describe('delay test jobs', () => {
  it('starts one backend job with de-duplicated node names', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      jobId: 'delay-1', state: 'running', names: ['a', 'b'], results: [],
    }), { status: 202, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(createDelayTest(['a', 'b', 'a'])).resolves.toMatchObject({ jobId: 'delay-1', state: 'running' })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(JSON.parse(String(fetchMock.mock.calls[0]![1]?.body))).toEqual({ names: ['a', 'b'] })
  })

  it('restores the latest backend job snapshot', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({
      jobId: 'delay-1', state: 'done', names: ['a'], results: [{ name: 'a', delay: 12, state: 'done' }],
    }), { status: 200, headers: { 'Content-Type': 'application/json' } })))

    await expect(loadDelayTestStatus()).resolves.toMatchObject({
      state: 'done', results: [{ name: 'a', delay: 12, state: 'done' }],
    })
  })

  it('subscribes to job completion independently of a page component', () => {
    vi.stubGlobal('EventSource', EventSourceMock)
    const update = vi.fn()
    const close = openDelayTestStream('delay/a', update)
    const source = EventSourceMock.instance!

    expect(source.url).toBe('/api/delays/delay%2Fa/stream')
    source.onmessage?.({ data: '{"jobId":"delay/a","state":"done","names":["a"],"results":[]}' } as MessageEvent<string>)
    expect(update).toHaveBeenCalledWith({ jobId: 'delay/a', state: 'done', names: ['a'], results: [] })
    close()
    expect(source.closed).toBe(true)
  })
})

it('sends the group and source identity for manual node tests', async () => {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ state: 'running', names: ['same'], results: [] }), { status: 202, headers: { 'Content-Type': 'application/json' } }))
  vi.stubGlobal('fetch', fetchMock)
  await createDelayTest(['same'], 'AI', 'profile-a')
  expect(JSON.parse(String(fetchMock.mock.calls[0]![1]?.body))).toEqual({ names: ['same'], group: 'AI', scope: 'profile-a' })
})

describe('submitted group progress', () => {
  const running: DelayTestStatus = { state: 'running', names: ['shared'], groups: ['AI'], targets: [
    { name: 'shared', group: 'AI', scope: 'profile-a', url: 'https://shared.example/' },
  ], results: [] }

  it('does not mark another group as submitted just because its nodes and URL overlap', () => {
    expect(delayTestGroupRunning(running, 'profile-a', 'AI')).toBe(true)
    expect(delayTestGroupRunning(running, 'profile-a', 'Video')).toBe(false)
    expect(delayTestGroupBatchRunning(running, 'profile-a')).toBe(false)
  })

  it('keeps every submitted batch group visible after identical targets were de-duplicated', () => {
    const batch = { ...running, groups: ['AI', 'Video'], groupBatch: true }
    expect(delayTestGroupRunning(batch, 'profile-a', 'Video')).toBe(true)
    expect(delayTestGroupRunning(batch, 'profile-a', 'Other')).toBe(false)
    expect(delayTestGroupBatchRunning(batch, 'profile-a')).toBe(true)
  })

  it('clears progress after completion and hides jobs from another subscription', () => {
    expect(delayTestGroupRunning({ ...running, state: 'done' }, 'profile-a', 'AI')).toBe(false)
    expect(delayTestGroupRunning(running, 'profile-b', 'AI')).toBe(false)
    expect(delayTestGroupBatchRunning({ ...running, groupBatch: true }, 'profile-b')).toBe(false)
  })

  it('uses exact target group identity when restoring an older job without submitted groups', () => {
    const legacy = { ...running, groups: undefined }
    expect(delayTestGroupRunning(legacy, 'profile-a', 'AI')).toBe(true)
    expect(delayTestGroupRunning(legacy, 'profile-a', 'Video')).toBe(false)
  })
})
