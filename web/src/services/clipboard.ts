// HTTP NAS pages may not expose the async Clipboard API. Keep the fallback
// inside the active modal so focus management cannot steal its selection.
export async function copyText(text: string): Promise<void> {
  if (globalThis.isSecureContext && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return
    } catch { /* Try the user-initiated legacy copy when permission is denied. */ }
  }
  const focused = document.activeElement
  const selection = document.getSelection()
  const ranges = selection ? Array.from({ length: selection.rangeCount }, (_, index) => selection.getRangeAt(index).cloneRange()) : []
  const field = document.createElement('textarea')
  field.value = text
  field.readOnly = true
  field.style.cssText = 'position:fixed;left:-10000px;top:0;opacity:0'
  const parent = focused instanceof HTMLElement ? focused.closest<HTMLElement>('[role="dialog"], dialog') : null
  ;(parent || document.body).appendChild(field)
  try {
    field.focus({ preventScroll: true })
    field.select()
    if (!document.execCommand('copy')) throw new Error('Clipboard copy failed')
  } finally {
    field.remove()
    if (focused instanceof HTMLElement && focused.isConnected) focused.focus({ preventScroll: true })
    if (selection) {
      selection.removeAllRanges()
      for (const range of ranges) selection.addRange(range)
    }
  }
}
