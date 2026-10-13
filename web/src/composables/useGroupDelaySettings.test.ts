import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useGroupDelaySettings } from './useGroupDelaySettings'
import type { GroupDelayContext } from '@/types/api'

const mocks = vi.hoisted(() => ({ api: vi.fn() }))
vi.mock('@/services/api', () => ({
  api: mocks.api,
  errorMessage: (cause: unknown) => cause instanceof Error ? cause.message : String(cause),
  jsonRequest: (method: string, body: unknown) => ({ method, body: JSON.stringify(body) }),
}))

function setup(mode: 'auto' | 'global' | 'custom' = 'auto') {
  const context = ref<GroupDelayContext>({ scope: 'profile-a', globalUrl: 'https://global.example/204', groups: {
    A: { mode, url: mode === 'global' ? 'https://global.example/204' : 'https://group.example/204', configUrl: 'https://group.example/204', source: mode === 'auto' ? 'config' : mode },
    B: { mode: 'auto', url: 'https://global.example/204', source: 'global' },
  } })
  mocks.api.mockImplementation(async (_path, request) => {
    const { group, mode: nextMode, url } = JSON.parse(request.body)
    const result: GroupDelayContext = JSON.parse(JSON.stringify(context.value))
    result.groups[group] = { ...result.groups[group]!, mode: nextMode, url: nextMode === 'custom' ? url : result.groups[group]?.configUrl || result.globalUrl, source: nextMode === 'custom' ? 'custom' : 'config' }
    return result
  })
  const settings = useGroupDelaySettings(context, value => { context.value = value })
  return { context, settings }
}

describe('inline group delay settings', () => {
  beforeEach(() => { mocks.api.mockReset() })

  it('opens an empty editor with the effective URL as its placeholder, then saves the typed URL', async () => {
    const { context, settings } = setup()
    await settings.toggle('A')
    expect(settings.isCustom('A')).toBe(true)
    expect(settings.url('A')).toBe('')
    expect(settings.defaultUrl('A')).toBe('https://group.example/204')
    expect(await settings.save('A')).toBe(true)
    expect(settings.error('A')).toBe('')
    expect(mocks.api).not.toHaveBeenCalled()
    expect(context.value.groups.A?.mode).toBe('auto')
    settings.edit('A', ' https://custom.example/204 ')
    expect(await settings.save('A')).toBe(true)
    expect(JSON.parse(mocks.api.mock.calls[0]![1].body)).toEqual({ scope: 'profile-a', group: 'A', mode: 'custom', url: 'https://custom.example/204' })
    expect(context.value.groups.A?.url).toBe('https://custom.example/204')
    expect(context.value.groups.B?.mode).toBe('auto')
  })

  it('keeps a saved custom URL as text and uses the original default only as a hint', () => {
    const { context, settings } = setup('custom')
    context.value.groups.A!.url = 'https://custom.example/204'
    expect(settings.url('A')).toBe('https://custom.example/204')
    expect(settings.defaultUrl('A')).toBe('https://group.example/204')
  })

  it('uses the global default as a hint and does not prefill it on reopening the editor', async () => {
    const { context, settings } = setup('global')
    await settings.toggle('A')
    expect(settings.url('A')).toBe('')
    expect(settings.defaultUrl('A')).toBe('https://global.example/204')
    expect(await settings.save('A')).toBe(true)
    expect(context.value.groups.A?.mode).toBe('global')
    expect(mocks.api).not.toHaveBeenCalled()
    await settings.toggle('A')
    await settings.toggle('A')
    expect(settings.url('A')).toBe('')
    expect(settings.defaultUrl('B')).toBe('https://global.example/204')
  })

  it('restores automatic URL selection when custom is switched off', async () => {
    const { context, settings } = setup('custom')
    await settings.toggle('A')
    expect(settings.isCustom('A')).toBe(false)
    expect(context.value.groups.A?.mode).toBe('auto')
    expect(context.value.groups.A?.url).toBe('https://group.example/204')
  })

  it.each(['', 'bad-url', 'ftp://example.com', 'https://user:password@example.com'])('blocks invalid custom URL %j before saving or testing', async value => {
    const { settings } = setup()
    settings.edit('A', value)
    expect(await settings.save('A')).toBe(false)
    expect(settings.error('A')).toBeTruthy()
    expect(mocks.api).not.toHaveBeenCalled()
  })

  it('awaits an in-flight blur save before a test and serializes other group saves', async () => {
    const { settings } = setup()
    let finish!: (value: GroupDelayContext) => void
    const response: GroupDelayContext = { scope: 'profile-a', globalUrl: 'https://global.example/204', groups: {
      A: { mode: 'custom', url: 'https://custom.example/204', source: 'custom' },
      B: { mode: 'auto', url: 'https://global.example/204', source: 'global' },
    } }
    mocks.api.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    settings.edit('A', 'https://custom.example/204')
    const first = settings.save('A')
    expect(settings.save('A')).toBe(first)
    settings.edit('B', 'https://other.example/204')
    const second = settings.save('B')
    await Promise.resolve()
    expect(mocks.api).toHaveBeenCalledOnce()
    finish(response)
    expect(await first).toBe(true)
    expect(await second).toBe(true)
    expect(settings.url('A')).toBe('https://custom.example/204')
    expect(settings.url('B')).toBe('https://other.example/204')
  })

  it('keeps the editor and previous saved URL when saving fails', async () => {
    const { context, settings } = setup('custom')
    mocks.api.mockRejectedValue(new Error('save failed'))
    settings.edit('A', 'https://custom.example/204')
    expect(await settings.save('A')).toBe(false)
    expect(settings.url('A')).toBe('https://custom.example/204')
    expect(context.value.groups.A?.url).toBe('https://group.example/204')
    await settings.toggle('A')
    expect(settings.isCustom('A')).toBe(true)
    expect(settings.error('A')).toBe('save failed')
  })

  it('does not replace a different subscription context with a late save response', async () => {
    const { context, settings } = setup()
    let finish!: (value: GroupDelayContext) => void
    mocks.api.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    settings.edit('A', 'https://custom.example/204')
    const saving = settings.save('A')
    await Promise.resolve()
    const original: GroupDelayContext = JSON.parse(JSON.stringify(context.value))
    context.value = { scope: 'profile-b', globalUrl: 'https://new.example/204', groups: {} }
    finish(original)
    expect(await saving).toBe(false)
    expect(context.value.scope).toBe('profile-b')
    expect(settings.isCustom('A')).toBe(false)
  })

  it('leaves an existing global preference intact unless the user changes it', async () => {
    const { context, settings } = setup('global')
    expect(await settings.save('A')).toBe(true)
    expect(context.value.groups.A?.mode).toBe('global')
    expect(mocks.api).not.toHaveBeenCalled()
  })
})
