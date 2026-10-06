/** Shared latency colours for measured results; timeout/error states are handled by callers. */
export function latencyClass(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return ''
  return value < 250 ? 'good' : value < 400 ? 'primary' : 'warn'
}
