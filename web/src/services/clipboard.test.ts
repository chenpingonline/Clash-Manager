import { afterEach, describe, expect, it, vi } from 'vitest'
import { copyText } from './clipboard'

afterEach(() => vi.unstubAllGlobals())
function environment(secure = false, modern?: ReturnType<typeof vi.fn>) {
  const modal = { appendChild: vi.fn() }
  class Element {
    isConnected = true
    focus = vi.fn()
    closest = vi.fn(() => modal)
  }
  const focused = new Element()
  const field = { value: '', readOnly: false, style: { cssText: '' }, focus: vi.fn(), select: vi.fn(), remove: vi.fn() }
  const range = {}
  const selection = { rangeCount: 1, getRangeAt: vi.fn(() => ({ cloneRange: () => range })), removeAllRanges: vi.fn(), addRange: vi.fn() }
  const exec = vi.fn(() => true)
  const body = { appendChild: vi.fn() }
  vi.stubGlobal('HTMLElement', Element)
  vi.stubGlobal('isSecureContext', secure)
  vi.stubGlobal('navigator', modern ? { clipboard: { writeText: modern } } : {})
  vi.stubGlobal('document', { activeElement: focused, getSelection: () => selection, createElement: () => field, execCommand: exec, body })
  return { modal, focused, field, selection, range, exec, body }
}
describe('HTTP-compatible clipboard copy', () => {
  it('copies the exact diagnostic JSON on HTTP without the modern API, inside the modal', async () => {
    const e = environment()
    const text = '{\n  "platform": "fnos",\n  "probe": {"usable": true}\n}'
    await copyText(text)
    expect(e.field.value).toBe(text)
    expect(e.modal.appendChild).toHaveBeenCalledWith(e.field)
    expect(e.body.appendChild).not.toHaveBeenCalled()
    expect(e.field.select).toHaveBeenCalledOnce()
    expect(e.exec).toHaveBeenCalledWith('copy')
    expect(e.field.remove).toHaveBeenCalledOnce()
    expect(e.focused.focus).toHaveBeenCalledWith({ preventScroll: true })
    expect(e.selection.addRange).toHaveBeenCalledWith(e.range)
  })
  it('uses the modern API in a secure context and falls back after rejection', async () => {
    const modern = vi.fn().mockResolvedValue(undefined)
    const e = environment(true, modern)
    await copyText('first')
    expect(modern).toHaveBeenCalledWith('first')
    expect(e.exec).not.toHaveBeenCalled()
    modern.mockRejectedValueOnce(new Error('denied'))
    await copyText('retry')
    expect(e.field.value).toBe('retry')
    expect(e.exec).toHaveBeenCalledOnce()
  })
  it('reports a rejected legacy copy and restores focus and selection', async () => {
    const e = environment()
    e.exec.mockReturnValue(false)
    await expect(copyText('diagnostic')).rejects.toThrow('Clipboard copy failed')
    expect(e.field.remove).toHaveBeenCalledOnce()
    expect(e.focused.focus).toHaveBeenCalledOnce()
    expect(e.selection.addRange).toHaveBeenCalledWith(e.range)
  })
})
