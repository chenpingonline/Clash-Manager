import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useStartupSession } from './useStartupSession'

const mocks = vi.hoisted(() => ({ api: vi.fn(), runtime: vi.fn() }))
vi.mock('@/services/api', async importOriginal => ({ ...await importOriginal<typeof import('@/services/api')>(), api: mocks.api }))
vi.mock('@/services/runtime', () => ({ loadRuntime: mocks.runtime }))

beforeEach(() => { vi.useFakeTimers(); mocks.api.mockReset(); mocks.runtime.mockReset() })
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks() })

describe('startup session', () => {
  it('renders fnOS immediately while runtime is pending and never checks Docker authentication', async () => {
    mocks.runtime.mockImplementation(() => new Promise(() => undefined))
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const state = useStartupSession(true)
    expect(state.ready.value).toBe(true)
    expect(state.loading.value).toBe(false)
    const pending = state.initialize()
    expect(state.ready.value).toBe(true)
    expect(mocks.api).not.toHaveBeenCalled()
    state.expire()
    expect(state.ready.value).toBe(true)
    await vi.advanceTimersByTimeAsync(10_000)
    await pending
    expect(warn).toHaveBeenCalledOnce()
    expect(state.ready.value).toBe(true)
    expect(state.error.value).toBe('')
    expect(vi.getTimerCount()).toBe(0)
  })
  it('does not show Docker content before both session and runtime have completed', async () => {
    mocks.api.mockResolvedValue({ required: true, authenticated: true })
    let finishRuntime: (() => void) | undefined
    mocks.runtime.mockImplementation(() => new Promise<void>(resolve => { finishRuntime = resolve }))
    const state = useStartupSession(false)
    const pending = state.initialize()
    await Promise.resolve()
    expect(state.ready.value).toBe(false)
    finishRuntime!()
    await pending
    expect(state.ready.value).toBe(true)
    expect(state.required.value).toBe(true)
    expect(state.loading.value).toBe(false)
    expect(vi.getTimerCount()).toBe(0)
  })
  it('shows Docker login for an unauthenticated session and accepts a successful login', async () => {
    mocks.api.mockResolvedValueOnce({ required: true, authenticated: false }).mockResolvedValueOnce({ ok: true })
    mocks.runtime.mockResolvedValue(undefined)
    const state = useStartupSession(false)
    await state.initialize()
    expect(state.ready.value).toBe(false)
    expect(state.required.value).toBe(true)
    expect(mocks.runtime).not.toHaveBeenCalled()
    state.password.value = 'test-password'
    await state.login()
    expect(mocks.api).toHaveBeenLastCalledWith('/api/auth/login', expect.objectContaining({ method: 'POST', signal: expect.any(AbortSignal) }))
    expect(state.ready.value).toBe(true)
    expect(state.password.value).toBe('')
    state.expire()
    expect(state.ready.value).toBe(false)
    expect(state.error.value).toBe('登录已过期，请重新登录')
    state.expire(true)
    expect(state.error.value).toBe('')
  })
  it.each(['session', 'runtime'])('ends a stalled Docker %s request and permits a retry', async stage => {
    const stalled = () => new Promise(() => undefined)
    mocks.api.mockImplementation(stage === 'session' ? stalled : () => Promise.resolve({ required: true, authenticated: true }))
    mocks.runtime.mockImplementation(stalled)
    const state = useStartupSession(false)
    const pending = state.initialize()
    await vi.advanceTimersByTimeAsync(10_000)
    await pending
    expect(state.ready.value).toBe(false)
    expect(state.loading.value).toBe(false)
    expect(state.error.value).toBe('连接超时，请检查服务状态后重试')
    const signal = mocks.api.mock.calls[0]![1].signal as AbortSignal
    expect(signal.aborted).toBe(true)
    mocks.api.mockResolvedValue({ required: true, authenticated: true })
    mocks.runtime.mockResolvedValue(undefined)
    await state.initialize()
    expect(state.ready.value).toBe(true)
    expect(state.error.value).toBe('')
    expect(vi.getTimerCount()).toBe(0)
  })
  it('ends a stalled login and never accepts a late result after expiry or disposal', async () => {
    mocks.api.mockResolvedValueOnce({ required: true, authenticated: false })
    const state = useStartupSession(false)
    await state.initialize()
    let finish: (() => void) | undefined
    mocks.api.mockImplementation(() => new Promise<void>(resolve => { finish = resolve }))
    const pending = state.login()
    await vi.advanceTimersByTimeAsync(10_000)
    await pending
    expect(state.busy.value).toBe(false)
    expect(state.error.value).toBe('连接超时，请检查服务状态后重试')
    finish!()
    await Promise.resolve()
    expect(state.ready.value).toBe(false)
    expect(mocks.runtime).not.toHaveBeenCalled()

    const another = state.login()
    state.expire(true)
    finish!()
    await another
    expect(state.ready.value).toBe(false)
    expect(state.error.value).toBe('')
    const last = state.initialize()
    const signal = mocks.api.mock.calls[mocks.api.mock.calls.length - 1]![1].signal as AbortSignal
    state.dispose()
    await last
    expect(signal.aborted).toBe(true)
    expect(state.ready.value).toBe(false)
    expect(vi.getTimerCount()).toBe(0)
  })
})
