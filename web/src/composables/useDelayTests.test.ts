import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  load: vi.fn(),
  streamUpdate: null as null | ((status: unknown) => void),
}))

vi.mock('@/services/delay-tests', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/services/delay-tests')>(),
  createDelayTest: mocks.create,
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
