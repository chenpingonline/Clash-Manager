import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ api: vi.fn() }))
vi.mock('@/services/api', () => ({ api: mocks.api }))

describe('shared app update information', () => {
  beforeEach(() => { vi.resetModules(); mocks.api.mockReset() })

  it('keeps the startup release visible when settings loads basic version information', async () => {
    const { useAppUpdateNotice } = await import('./useAppUpdateNotice')
    mocks.api.mockImplementation(async (path: string) => path === '/api/settings' ? { notifyAppUpdates: true } : { currentVersion: '1.3.14', sourceConfigured: true, updateAvailable: true, latest: { tag: 'v1.3.15', asset: { url: 'https://github.com/update.fpk' } } })
    const startup = useAppUpdateNotice()
    await startup.initialize()
    const settings = useAppUpdateNotice()
    settings.applyResult({ currentVersion: '1.3.14', sourceConfigured: true, platform: 'x86', updateAvailable: null } as never)
    expect(settings.available.value).toBe(true)
    expect(settings.info.value.latest?.tag).toBe('v1.3.15')
    expect(settings.info.value.platform).toBe('x86')
    expect(mocks.api.mock.calls.filter(call => call[0] === '/api/app/check-update')).toHaveLength(1)
  })

  it('updates settings when the startup request finishes after settings loads', async () => {
    const { useAppUpdateNotice } = await import('./useAppUpdateNotice')
    let finish!: (value: unknown) => void
    mocks.api.mockReturnValue(new Promise(resolve => { finish = resolve }))
    const startup = useAppUpdateNotice(), settings = useAppUpdateNotice()
    const pending = startup.check()
    settings.applyResult({ currentVersion: '1.3.14', sourceConfigured: true })
    const second = settings.check()
    finish({ currentVersion: '1.3.14', sourceConfigured: true, updateAvailable: true, latest: { tag: 'v1.3.15' } })
    await Promise.all([pending, second])
    expect(settings.info.value.latest?.tag).toBe('v1.3.15')
    expect(mocks.api).toHaveBeenCalledOnce()
  })

  it('clears a detected update when the installed version changes and honors notice preference', async () => {
    const { useAppUpdateNotice } = await import('./useAppUpdateNotice')
    const notice = useAppUpdateNotice()
    notice.applyResult({ currentVersion: '1.3.14', sourceConfigured: true, updateAvailable: true, latest: { tag: 'v1.3.15' } })
    notice.setEnabled(false)
    expect(notice.available.value).toBe(false)
    expect(notice.info.value.updateAvailable).toBe(true)
    notice.setEnabled(true)
    notice.applyResult({ currentVersion: '1.3.15', sourceConfigured: true })
    expect(notice.available.value).toBe(false)
    expect(notice.info.value.latest).toBeUndefined()
  })
})
