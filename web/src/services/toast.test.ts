import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

beforeEach(() => { vi.resetModules(); vi.useFakeTimers(); vi.stubGlobal('window', { setTimeout }) })
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals() })

describe('configuration error notifications', () => {
  it('keeps validation failures visible until dismissed', async () => {
    const { notify, toasts, dismissToast } = await import('./toast')
    notify('Mihomo 配置校验失败: unknown output', true)
    expect(toasts.value[0]?.configError).toBe(true)
    vi.advanceTimersByTime(60_000)
    expect(toasts.value).toHaveLength(1)
    dismissToast(toasts.value[0]!.id)
    expect(toasts.value).toHaveLength(0)
  })
  it('retains automatic expiry for success and unrelated failures', async () => {
    const { notify, toasts } = await import('./toast')
    notify('已保存'); notify('HTTP 503', true)
    expect(toasts.value.every(item => !item.configError)).toBe(true)
    vi.advanceTimersByTime(3200)
    expect(toasts.value).toHaveLength(0)
  })
  it('replaces an earlier validation error without removing unrelated notifications', async () => {
    const { notify, toasts } = await import('./toast')
    notify('已保存')
    notify('Mihomo 配置校验失败: first', true)
    notify('Mihomo 配置校验失败: second', true)
    expect(toasts.value.map(item => item.text)).toEqual(['已保存', 'Mihomo 配置校验失败: second'])
  })
  it('clears resolved configuration failures while preserving unrelated notifications', async () => {
    const { notify, toasts, dismissConfigErrors } = await import('./toast')
    notify('HTTP 503', true)
    notify('Mihomo 配置校验失败: first', true)
    dismissConfigErrors()
    expect(toasts.value.map(item => item.text)).toEqual(['HTTP 503'])
  })
})
