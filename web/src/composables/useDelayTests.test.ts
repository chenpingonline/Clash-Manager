import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  createGroups: vi.fn(),
  load: vi.fn(),
  streamUpdate: null as null | ((status: unknown) => void),
}))

vi.mock('@/services/delay-tests', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/services/delay-tests')>(),
  createDelayTest: mocks.create,
  createGroupDelayTest: mocks.createGroups,
  loadDelayTestStatus: mocks.load,
  openDelayTestStream: vi.fn((_jobId: string, update: (status: unknown) => void) => {
    mocks.streamUpdate = update
    return vi.fn()
  }),
}))

describe('useDelayTests', () => {
  beforeEach(() => {
    mocks.create.mockReset()
    mocks.load.mockReset()
    mocks.streamUpdate = null
  })

  it('shares a restored running job across page composable instances', async () => {
    mocks.load.mockResolvedValue({ jobId: 'delay-1', state: 'running', names: ['node-a'], results: [] })
    const { useDelayTests } = await import('./useDelayTests')
    const proxiesPage = useDelayTests()
    await proxiesPage.restore()

    expect(proxiesPage.testing.value).toBe(true)
    expect(proxiesPage.delays.get('node-a')).toEqual({ value: 0, state: 'testing' })

    const dashboardPage = useDelayTests()
    expect(dashboardPage.testing.value).toBe(true)
    expect(dashboardPage.delays.get('node-a')?.state).toBe('testing')

    mocks.streamUpdate?.({
      jobId: 'delay-1', state: 'done', names: ['node-a'],
      results: [{ name: 'node-a', delay: 24, state: 'done' }],
    })
    expect(dashboardPage.testing.value).toBe(false)
    expect(proxiesPage.delays.get('node-a')).toEqual({ value: 24, state: 'done' })
  })

  it('keeps the precise failure reason and clears it on the next measurement', async () => {
    const { useDelayTests } = await import('./useDelayTests')
    const tests = useDelayTests()
    mocks.load.mockResolvedValue({ state: 'done', names: ['provider-node'], results: [
      { name: 'provider-node', state: 'error', error: 'Mihomo 404: Not Found' },
    ] })
    await tests.restore()
    expect(tests.delays.get('provider-node')).toEqual({ value: 0, state: 'error', error: 'Mihomo 404: Not Found' })
    mocks.load.mockResolvedValue({ state: 'done', names: ['provider-node'], results: [
      { name: 'provider-node', state: 'done', delay: 32 },
    ] })
    await tests.restore()
    expect(tests.delays.get('provider-node')).toEqual({ value: 32, state: 'done' })
  })
})

describe('immediate delay submission feedback', () => {
  const context = { scope: 'profile-a', globalUrl: 'https://shared.example/', groups: {
    AI: { mode: 'auto' as const, source: 'global' as const, url: 'https://shared.example/' },
    Video: { mode: 'auto' as const, source: 'global' as const, url: 'https://shared.example/' },
  } }
  const done = { state: 'done' as const, names: ['shared'], results: [] }
  beforeEach(() => {
    vi.resetModules()
    mocks.create.mockReset()
    mocks.createGroups.mockReset()
    mocks.load.mockReset()
  })

  it('immediately marks only the clicked group while URL saving and submission are pending', async () => {
    const { useDelayTests } = await import('./useDelayTests')
    const tests = useDelayTests()
    tests.setContext(context)
    let finishSave!: (saved: boolean) => void
    const saving = new Promise<boolean>(resolve => { finishSave = resolve })
    let accept!: (status: typeof done) => void
    mocks.create.mockReturnValue(new Promise(resolve => { accept = resolve }))
    const completion = tests.start(['shared'], 'AI', () => saving)
    expect(tests.testing.value).toBe(true)
    expect(tests.status.value.groups).toEqual(['AI'])
    expect(tests.status.value.groupBatch).toBe(false)
    expect(mocks.create).not.toHaveBeenCalled()
    await expect(tests.start(['shared'], 'Video')).rejects.toThrow('已有测速任务正在进行')
    finishSave(true)
    await Promise.resolve()
    expect(mocks.create).toHaveBeenCalledWith(['shared'], 'AI', 'profile-a')
    expect(tests.testing.value).toBe(true)
    // A pending submission must not erase previous measurements or leave testing nodes on failure.
    expect(tests.get('shared', 'AI')).toBeUndefined()
    accept(done)
    await completion
    expect(tests.testing.value).toBe(false)
  })

  it('restores controls on a failed save or rejected request, and allows retry', async () => {
    const { useDelayTests } = await import('./useDelayTests')
    const tests = useDelayTests()
    tests.setContext(context)
    expect(await tests.start(['shared'], 'AI', async () => false)).toBeNull()
    expect(tests.testing.value).toBe(false)
    expect(mocks.create).not.toHaveBeenCalled()
    mocks.create.mockRejectedValueOnce(new Error('request failed')).mockResolvedValueOnce(done)
    await expect(tests.start(['shared'], 'AI')).rejects.toThrow('request failed')
    expect(tests.testing.value).toBe(false)
    expect(tests.status.value.state).toBe('idle')
    await tests.start(['shared'], 'AI')
    expect(mocks.create).toHaveBeenCalledTimes(2)
  })

  it('shows all submitted groups while a batch request is pending, then follows the accepted job', async () => {
    const { useDelayTests } = await import('./useDelayTests')
    const tests = useDelayTests()
    tests.setContext(context)
    let accept!: (status: unknown) => void
    mocks.createGroups.mockReturnValue(new Promise(resolve => { accept = resolve }))
    const groups = [{ group: 'AI', names: ['shared'] }, { group: 'Video', names: ['shared'] }]
    const completion = tests.startGroups(groups)
    expect(tests.status.value.groups).toEqual(['AI', 'Video'])
    expect(tests.status.value.groupBatch).toBe(true)
    accept({ ...done, jobId: 'accepted', state: 'running', groups: ['AI', 'Video'], groupBatch: true })
    await Promise.resolve()
    await Promise.resolve()
    expect(tests.status.value.jobId).toBe('accepted')
    expect(tests.testing.value).toBe(true)
    mocks.streamUpdate?.({ ...done, jobId: 'accepted' })
    await completion
    expect(tests.testing.value).toBe(false)
  })

  it('does not submit to a different subscription after saving finishes', async () => {
    const { useDelayTests } = await import('./useDelayTests')
    const tests = useDelayTests()
    tests.setContext(context)
    await expect(tests.start(['shared'], 'AI', async () => {
      tests.setContext({ ...context, scope: 'profile-b' })
      return true
    })).rejects.toThrow('当前配置已切换，请刷新后重试')
    expect(mocks.create).not.toHaveBeenCalled()
    expect(tests.testing.value).toBe(false)
  })
})

it('isolates the same node by URL, status expectation and subscription, including late completion', async () => {
  const { useDelayTests } = await import('./useDelayTests')
  const tests = useDelayTests()
  const context = { scope: 'profile-a', globalUrl: 'https://global.example/', groups: {
    AI: { mode: 'auto' as const, source: 'config' as const, url: 'https://ai.example/', expected: '200' },
    Video: { mode: 'custom' as const, source: 'custom' as const, url: 'https://video.example/' },
  } }
  tests.setContext(context)
  mocks.load.mockResolvedValue({ state: 'done', names: [], results: [
    { name: 'shared', scope: 'profile-a', url: 'https://ai.example/', expected: '200', state: 'done', delay: 20 },
    { name: 'shared', scope: 'profile-a', url: 'https://video.example/', state: 'timeout' },
  ] })
  await tests.restore()
  expect(tests.get('shared', 'AI')?.value).toBe(20)
  expect(tests.get('shared', 'Video')?.state).toBe('timeout')
  tests.setContext({ ...context, scope: 'profile-b' })
  expect(tests.get('shared', 'AI')).toBeUndefined()
  // A completion for A must not appear in the currently displayed B.
  mocks.load.mockResolvedValue({ state: 'done', names: ['shared'], targets: [
    { name: 'shared', scope: 'profile-a', url: 'https://ai.example/', expected: '200' },
  ], results: [{ name: 'shared', scope: 'profile-a', url: 'https://ai.example/', expected: '200', state: 'done', delay: 30 }] })
  await tests.restore()
  expect(tests.get('shared', 'AI')).toBeUndefined()
  tests.setContext(context)
  expect(tests.get('shared', 'AI')?.value).toBe(30)
  tests.setContext({ ...context, groups: { ...context.groups, AI: { ...context.groups.AI, expected: '204' } } })
  expect(tests.get('shared', 'AI')).toBeUndefined()
})
