import { describe, expect, it } from 'vitest'
import { containsRule, normalizeRules } from './rules'

describe('rule presentation', () => {
  it('normalizes API rules with stable display line numbers', () => {
    expect(normalizeRules([
      { index: 99, type: 'DomainSuffix', payload: 'example.com', proxy: 'DIRECT' },
      { type: 'IPCIDR', payload: '10.0.0.0/8', proxy: 'LAN' },
    ])).toEqual([
      { lineNo: 1, index: 99, disabled: false, canToggle: false, type: 'DomainSuffix', payload: 'example.com', proxy: 'DIRECT' },
      { lineNo: 2, index: null, disabled: false, canToggle: false, type: 'IPCIDR', payload: '10.0.0.0/8', proxy: 'LAN' },
    ])
  })

  it('retains the core index and disabled state when filtering rules', () => {
    const rules = normalizeRules([
      { index: 0, type: 'Domain', payload: 'first.example', proxy: 'DIRECT', extra: { disabled: false } },
      { index: 7, type: 'Domain', payload: 'target.example', proxy: 'DIRECT', extra: { disabled: true } },
    ]).filter(rule => containsRule(rule, 'target'))
    expect(rules[0]).toMatchObject({ index: 7, lineNo: 2, disabled: true, canToggle: true })
  })

  it('searches payload, type, and target case-insensitively', () => {
    const [rule] = normalizeRules([{ type: 'DomainSuffix', payload: 'Example.COM', proxy: 'DIRECT' }])
    expect(containsRule(rule!, 'example')).toBe(true)
    expect(containsRule(rule!, 'domainsuffix')).toBe(true)
    expect(containsRule(rule!, 'direct')).toBe(true)
    expect(containsRule(rule!, 'reject')).toBe(false)
  })
})
