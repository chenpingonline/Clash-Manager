import { describe, expect, it } from 'vitest'
import { nextSelectIndex, searchSelectIndex } from './select-options'

describe('custom select navigation', () => {
  const options = [{ value: '', label: 'Choose', disabled: true }, { value: 12, label: '12 hours' }, { value: 24, label: '24 hours', disabled: true }, { value: 72, label: '3 days' }]
  it('skips disabled options and wraps in both directions', () => {
    expect(nextSelectIndex(options, -1, 1)).toBe(1)
    expect(nextSelectIndex(options, 1, 1)).toBe(3)
    expect(nextSelectIndex(options, 3, 1)).toBe(1)
    expect(nextSelectIndex(options, 1, -1)).toBe(3)
    expect(nextSelectIndex(options, 0, -1)).toBe(3)
  })
  it('handles empty and entirely disabled menus', () => {
    expect(nextSelectIndex([], -1, 1)).toBe(-1)
    expect(nextSelectIndex([{ value: 'mips', label: 'mips', disabled: true }], 0, -1)).toBe(-1)
  })
  it('searches labels without selecting disabled entries or converting numeric values', () => {
    expect(searchSelectIndex(options, '24', 1)).toBe(-1)
    const index = searchSelectIndex(options, '3 D', 1)
    expect(index).toBe(3)
    expect(options[index]?.value).toBe(72)
  })
})
