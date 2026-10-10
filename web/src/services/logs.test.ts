import { describe, expect, it } from 'vitest'
import { containsLog, highlightParts, normalizeLog, plainCoreOutput } from './logs'

describe('log presentation', () => {
  it('filters case-insensitively and normalizes warning', () => {
    const item = normalizeLog({ time: '2026-09-05T11:00:00Z', level: 'warning', message: 'ChatGPT.com [a+b] <img>' })
    expect(item.level).toBe('warn')
    expect(containsLog(item, 'chatgpt')).toBe(true)
    expect(containsLog(item, 'missing')).toBe(false)
  })

  it('highlights literal metacharacters without producing HTML', () => {
    expect(highlightParts('ChatGPT.com [a+b] <img>', '[a+b]')).toEqual([
      { text: 'ChatGPT.com ', match: false },
      { text: '[a+b]', match: true },
      { text: ' <img>', match: false },
    ])
  })

  it('removes terminal controls while preserving literal diagnostic text', () => {
    expect(plainCoreOutput('\u001b[31mpanic: <script>alert(1)</script>\u001b[0m\tstack\r\n')).toBe('panic: <script>alert(1)</script>\tstack\n')
    expect(plainCoreOutput('\u001b]8;;https://example.com\u0007link\u001b]8;;\u001b\\')).toBe('link')
  })
})

it('shows the calendar date and includes the whole selected end date', async () => {
 const { displayLogTime, logDateBoundary } = await import('./logs')
 const start = new Date(logDateBoundary('2026-10-09'))
 const end = new Date(logDateBoundary('2026-10-09', true))
 expect(start.getDate()).toBe(9); expect(start.getHours()).toBe(0)
 expect(end.getDate()).toBe(10); expect(end.getHours()).toBe(0)
 expect(displayLogTime('2026-10-09T12:00:00Z')).toContain('2026')
 expect(logDateBoundary('')).toBe('')
})
