import { afterEach, describe, expect, it, vi } from 'vitest'
const json = (body: unknown) => new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })
afterEach(() => { vi.unstubAllGlobals(); vi.resetModules() })
describe('bypass page preference', () => {
  it('loads disabled preferences before allowing page checks', async () => {
    const preference = (await import('./useBypassPagePreference')).useBypassPagePreference()
    expect(preference.ready.value).toBe(false)
    expect(preference.enabled.value).toBe(false)
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(json({ bypassPageEnabled: false })))
    await preference.initialize()
    expect(preference.ready.value).toBe(true)
    expect(preference.enabled.value).toBe(false)
  })
  it('defaults to hidden for settings without the new field', async () => {
    const preference = (await import('./useBypassPagePreference')).useBypassPagePreference()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(json({})))
    await preference.initialize()
    expect(preference.enabled.value).toBe(false)
  })
  it('preserves an explicitly enabled preference', async () => {
    const preference = (await import('./useBypassPagePreference')).useBypassPagePreference()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(json({ bypassPageEnabled: true })))
    await preference.initialize()
    expect(preference.enabled.value).toBe(true)
  })
  it('does not mark failed reads as ready', async () => {
    const preference = (await import('./useBypassPagePreference')).useBypassPagePreference()
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))
    await expect(preference.initialize()).rejects.toThrow('offline')
    expect(preference.ready.value).toBe(false)
  })
  it('never overwrites a newer saved preference with a stale startup response', async () => {
    const preference = (await import('./useBypassPagePreference')).useBypassPagePreference()
    let resolve!: (value: Response) => void
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(done => { resolve = done })))
    const pending = preference.initialize()
    preference.sync({ bypassPageEnabled: false })
    resolve(json({ bypassPageEnabled: true }))
    await pending
    expect(preference.enabled.value).toBe(false)
    expect(preference.ready.value).toBe(true)
  })
})
