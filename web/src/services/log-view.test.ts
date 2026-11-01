import { describe, expect, it } from 'vitest'
import { appendLogBatch, isLogBottom, validLogLimit } from './log-view'

describe('bounded log display', () => {
  it('defaults invalid and missing preferences to 100 lines', () => {
    for (const value of [null, undefined, '', 'garbage', -1, 10000]) expect(validLogLimit(value)).toBe(100)
    expect(validLogLimit('200')).toBe(200)
  })
  it('retains newest records across bursts without changing surviving identities', () => {
    const old = [{ key: 1 }, { key: 2 }]
    const next = appendLogBatch(old, [{ key: 3 }, { key: 4 }], 3)
    expect(next.map(row => row.key)).toEqual([2, 3, 4])
    expect(next[0]).toBe(old[1])
    expect(old).toHaveLength(2)
    expect(appendLogBatch(next, Array.from({ length: 1000 }, (_, key) => ({ key: key + 5 })), 100)).toHaveLength(100)
  })
  it('follows only when near the bottom, including an empty view', () => {
    expect(isLogBottom(0, 400, 0)).toBe(true)
    expect(isLogBottom(580, 400, 1000)).toBe(true)
    expect(isLogBottom(200, 400, 1000)).toBe(false)
  })
})
