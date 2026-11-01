import { describe, expect, it, vi } from 'vitest'
import { createSettingsLoader, settingsResources, type SettingsResource } from './settings-loading'
function fixture() {
  const loaders = Object.fromEntries(['system', 'manager', 'network', 'environment', 'update', 'icons', 'geo', 'logDisplay'].map(key => [key, vi.fn(async (_signal: AbortSignal) => {})])) as Record<SettingsResource, ReturnType<typeof vi.fn<(signal: AbortSignal) => Promise<void>>>>
  return { loaders, loader: createSettingsLoader(loaders) }
}
describe('settings section loading', () => {
  it('opens core management without network, icons, GEO or update reads', async () => {
    const { loaders, loader } = fixture()
    await loader.load(settingsResources('core', 'manage'))
    expect(loaders.system).toHaveBeenCalledOnce(); expect(loaders.manager).toHaveBeenCalledOnce()
    for (const key of ['network', 'environment', 'update', 'icons', 'geo', 'logDisplay'] as const) expect(loaders[key]).not.toHaveBeenCalled()
  })
  it('keeps another tab usable while network is stalled, and deduplicates shared reads', async () => {
    const { loaders, loader } = fixture()
    let finish!: () => void
    loaders.network.mockImplementation(() => new Promise<void>(resolve => { finish = resolve }))
    const network = loader.load(settingsResources('network', ''))
    const dns = loader.load(settingsResources('dns', ''))
    await loader.load(settingsResources('core', 'manage'))
    expect(loader.state.system.ready).toBe(true); expect(loader.state.network.pending).toBe(true)
    expect(loaders.network).toHaveBeenCalledOnce()
    finish(); await Promise.all([network, dns])
    await loader.load(settingsResources('tun', ''))
    expect(loaders.network).toHaveBeenCalledOnce()
    loaders.network.mockResolvedValue(undefined)
    await loader.load(['network'], true); expect(loaders.network).toHaveBeenCalledTimes(2)
  })
  it('isolates failures and allows retry without exposing a default form', async () => {
    const { loaders, loader } = fixture()
    loaders.geo.mockRejectedValueOnce(new Error('GEO unavailable'))
    await loader.load(settingsResources('core', 'geo'))
    expect(loader.state.geo).toEqual({ ready: false, pending: false, error: 'GEO unavailable' })
    await loader.load(settingsResources('core', 'delay'))
    expect(loader.state.manager.ready).toBe(true)
    await loader.load(['geo']); expect(loader.state.geo.ready).toBe(true); expect(loader.state.geo.error).toBe('')
  })
  it('aborts outstanding reads on leaving the page', async () => {
    const { loaders, loader } = fixture()
    let signal!: AbortSignal
    loaders.network.mockImplementation(value => new Promise((_resolve, reject) => {
      signal = value; value.addEventListener('abort', () => reject(new Error('aborted')), { once: true })
    }))
    const task = loader.load(['network']); await Promise.resolve()
    loader.dispose(); expect(signal.aborted).toBe(true); await task
    await loader.load(['manager']); expect(loaders.manager).not.toHaveBeenCalled()
  })
})
