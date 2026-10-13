import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  platform: 'fnos',
  standalone: false,
  beforeUnmount: undefined as (() => void) | undefined,
  mounted: undefined as (() => void) | undefined,
  getPlatformConfig: vi.fn(),
  on: vi.fn(),
  off: vi.fn(),
}))

vi.mock('@/services/runtime', () => ({ runtime: { get value() { return { platform: mocks.platform } } } }))

vi.mock('vue', () => ({
  onMounted: (callback: () => void) => { mocks.mounted = callback },
  onBeforeUnmount: (callback: () => void) => { mocks.beforeUnmount = callback },
}))

vi.mock('@trimjs/web-app', () => ({
  TrimApp: class {
    isWeb = true
    isStandaloneWeb = mocks.standalone
    getPlatformConfig = mocks.getPlatformConfig
    $on = mocks.on
    $off = mocks.off
  },
}))

import { useFnosTheme } from './useFnosTheme'

describe('useFnosTheme', () => {
  const media = {
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }
  const root = {
    dataset: {} as Record<string, string>,
    style: {} as Record<string, string>,
  }

  beforeEach(() => {
    mocks.platform = 'fnos'
    mocks.standalone = false
    mocks.beforeUnmount = undefined
    mocks.mounted = undefined
    mocks.getPlatformConfig.mockReset()
    mocks.on.mockReset().mockResolvedValue(undefined)
    mocks.off.mockReset().mockResolvedValue(undefined)
    media.matches = false
    media.addEventListener.mockReset()
    media.removeEventListener.mockReset()
    root.dataset = {}
    root.style = {}
    vi.stubGlobal('matchMedia', vi.fn(() => media))
    vi.stubGlobal('document', { documentElement: root })
    vi.stubGlobal('localStorage', { setItem: vi.fn() })
  })

  afterEach(() => vi.unstubAllGlobals())

  it('reads the initial host theme and subscribes to real-time changes', async () => {
    mocks.getPlatformConfig.mockResolvedValue({ theme: 'dark' })
    useFnosTheme()
    mocks.mounted?.()
    await vi.waitFor(() => expect(mocks.on).toHaveBeenCalledOnce())

    expect(root.dataset.theme).toBe('dark')
    expect(mocks.on).toHaveBeenCalledWith('os/theme', expect.any(Function))

    const callback = mocks.on.mock.calls[0]?.[1] as (theme: unknown) => void
    callback({ theme: { theme: 'light' } })
    expect(root.dataset.theme).toBe('light')
    expect(localStorage.setItem).toHaveBeenLastCalledWith('clash-fnos-host-theme', 'light')
  })

  it('keeps the cached host theme until the host responds despite an opposite browser preference', async () => {
    root.dataset = { theme: 'light', bootThemeSource: 'host-cache' }
    media.matches = true
    let resolveConfig!: (config: { theme: string }) => void
    mocks.getPlatformConfig.mockImplementation(() => new Promise(resolve => { resolveConfig = resolve }))
    useFnosTheme()
    mocks.mounted?.()
    expect(root.dataset.theme).toBe('light')
    const changed = media.addEventListener.mock.calls[0]?.[1] as () => void
    changed()
    expect(root.dataset.theme).toBe('light')
    resolveConfig({ theme: 'dark' })
    await vi.waitFor(() => expect(root.dataset.theme).toBe('dark'))
    media.matches = false
    changed()
    expect(root.dataset.theme).toBe('dark')
  })

  it.each(['rejected', 'invalid'])('falls back from the cache when host configuration is %s', async (result) => {
    root.dataset = { theme: 'dark', bootThemeSource: 'host-cache' }
    if (result === 'rejected') mocks.getPlatformConfig.mockRejectedValue(new Error('unavailable'))
    else mocks.getPlatformConfig.mockResolvedValue({ theme: 'unknown' })
    useFnosTheme()
    mocks.mounted?.()
    await vi.waitFor(() => expect(root.dataset.theme).toBe('light'))
    expect(root.dataset.fnosThemeMode).toBe('system')
  })

  it('does not let blocked storage break live host theme updates', async () => {
    vi.stubGlobal('localStorage', { setItem: () => { throw new Error('blocked') } })
    mocks.getPlatformConfig.mockResolvedValue({ theme: 'dark' })
    useFnosTheme()
    mocks.mounted?.()
    await vi.waitFor(() => expect(mocks.on).toHaveBeenCalledOnce())
    expect(root.dataset.theme).toBe('dark')
  })

  it('ignores the embedded cache in a standalone browser', async () => {
    root.dataset = { theme: 'dark', bootThemeSource: 'host-cache' }
    mocks.standalone = true
    mocks.getPlatformConfig.mockRejectedValue(new Error('standalone'))
    useFnosTheme()
    mocks.mounted?.()
    expect(root.dataset.theme).toBe('light')
    await Promise.resolve()
    expect(localStorage.setItem).not.toHaveBeenCalled()
  })

  it('uses the browser theme only when the host SDK is unavailable', async () => {
    mocks.getPlatformConfig.mockRejectedValue(new Error('standalone'))
    useFnosTheme()
    mocks.mounted?.()
    await Promise.resolve()

    expect(root.dataset.theme).toBe('light')
    expect(root.dataset.fnosThemeMode).toBe('system')
    expect(mocks.on).not.toHaveBeenCalled()
  })

  it.each(['docker', 'linux'])('uses browser preferences in %s without contacting the host SDK', (platform) => {
    mocks.platform = platform
    media.matches = true
    useFnosTheme()
    mocks.mounted?.()
    expect(root.dataset.theme).toBe('dark')
    expect(mocks.getPlatformConfig).not.toHaveBeenCalled()
    expect(mocks.on).not.toHaveBeenCalled()
  })

  it('unsubscribes from the host event when unmounted', async () => {
    mocks.getPlatformConfig.mockResolvedValue({ theme: 'dark' })
    useFnosTheme()
    mocks.mounted?.()
    await vi.waitFor(() => expect(mocks.on).toHaveBeenCalledOnce())
    const callback = mocks.on.mock.calls[0]?.[1]

    mocks.beforeUnmount?.()

    expect(media.removeEventListener).toHaveBeenCalledWith('change', expect.any(Function))
    expect(mocks.off).toHaveBeenCalledWith('os/theme', callback)
  })
})
