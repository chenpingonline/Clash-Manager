import { describe, expect, it } from 'vitest'
import { logMessageParts } from './log-highlight'

describe('log syntax highlighting', () => {
  it('recognizes connection details without changing the message', () => {
    const message = '[UDP] 198.18.0.1:57203 --> [fd00::1]:53 match RuleSet(geoip-private) using 🎯Direct[DIRECT]'
    const parts = logMessageParts(message)
    expect(parts.map(part => part.text).join('')).toBe(message)
    expect(parts.filter(part => part.kind).map(({ text, kind }) => ({ text, kind }))).toEqual([
      { text: '[UDP]', kind: 'protocol' },
      { text: '198.18.0.1:57203', kind: 'address' },
      { text: '[fd00::1]:53', kind: 'address' },
      { text: 'RuleSet(geoip-private)', kind: 'rule' },
      { text: '🎯Direct[DIRECT]', kind: 'policy' },
    ])
  })

  it('keeps search matches that span protocol and address boundaries', () => {
    const message = '[TCP] 192.168.1.88:7890 --> EXAMPLE.com:443 timeout'
    const query = '[tcp] 192.168.1.88:7890'
    const parts = logMessageParts(message, query)
    expect(parts.filter(part => part.match).map(part => part.text).join('')).toBe('[TCP] 192.168.1.88:7890')
    expect(parts.some(part => part.text === 'EXAMPLE.com:443' && part.kind === 'address')).toBe(true)
    expect(parts.some(part => part.text === 'timeout' && part.kind === 'error')).toBe(true)
    expect(parts.map(part => part.text).join('')).toBe(message)
  })

  it('treats HTML and search metacharacters as literal text', () => {
    const message = '<img onerror="alert(1)"> [a+b] failed\t\n  panic: refused'
    const parts = logMessageParts(message, '[a+b]')
    expect(parts.map(part => part.text).join('')).toBe(message)
    expect(parts.filter(part => part.match).map(part => part.text)).toEqual(['[a+b]'])
    expect(parts.some(part => part.text === 'failed' && part.kind === 'error')).toBe(true)
    expect(logMessageParts('plain text')[0]).toEqual({ text: 'plain text', kind: '', match: false })
    expect(logMessageParts('')).toEqual([])
  })

  it('highlights raw Core severity fields and preserves stack trace indentation', () => {
    const message = 'time="2026-10-11T19:00:00+08:00" level=warning msg="failed to dial"\n\tstack: 0x1234'
    const parts = logMessageParts(message)
    expect(parts.some(part => part.text === 'level=warning' && part.kind === 'warn')).toBe(true)
    expect(parts.some(part => part.text === 'failed' && part.kind === 'error')).toBe(true)
    expect(parts.map(part => part.text).join('')).toBe(message)
    expect(logMessageParts('level=error connection refused').filter(part => part.kind === 'error').map(part => part.text)).toEqual(['level=error', 'refused'])
  })

  it('preserves repeated literal search matches inside a highlighted policy', () => {
    const message = '[TCP] using failure[failed-failed]'
    const parts = logMessageParts(message, 'failed')
    expect(parts.filter(part => part.match).map(part => part.text)).toEqual(['failed', 'failed'])
    expect(parts.map(part => part.text).join('')).toBe(message)
    expect(parts.find(part => part.text === 'failure')?.kind).toBe('error')
  })
})
