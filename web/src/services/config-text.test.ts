import { describe, expect, it } from 'vitest'
import { configMatchLine, configMatchRange, configSearchIndex, configTextDocument } from './config-text'

describe('configuration text navigation', () => {
  it('preserves indentation, blank lines and final newline, including CRLF files', () => {
    expect(configTextDocument('rules:\r\n  - MATCH,DIRECT\r\n\r\n').lines).toEqual(['rules:', '  - MATCH,DIRECT', '', ''])
    expect(configTextDocument('abc\n\t中文').widthLine).toBe('\t中文')
  })

  it('counts literal, case insensitive matches and maps each occurrence to its original line', () => {
    const index = configSearchIndex(['[a+b] [A+B]', '', 'none', '[a+b]'], '[a+b]')
    expect(index.count).toBe(3)
    expect([1, 2, 3].map(match => configMatchLine(index, match))).toEqual([0, 0, 3])
    expect(configMatchLine(index, 0)).toBe(-1)
    expect(configMatchLine(index, 4)).toBe(-1)
    expect(configSearchIndex(['abc'], '  ').count).toBe(0)
  })

  it('finds matches outside the rendered viewport in large configurations', () => {
    const lines = Array.from({ length: 30_000 }, (_, line) => `  - DOMAIN-SUFFIX,rule${line}.example.com,DIRECT`)
    const index = configSearchIndex(lines, 'EXAMPLE.COM')
    expect(index.count).toBe(30_000)
    expect(configMatchLine(index, 30_000)).toBe(29_999)
    expect(configMatchLine(configSearchIndex(lines, 'rule29999.'), 1)).toBe(29_999)
  })

  it('locates the last occurrence in a huge inline list, including normalized CRLF offsets', () => {
    const payload = Array.from({ length: 40_000 }, (_, i) => `+.rule${i}.example.com`).join(', ')
    const document = configTextDocument(`rules:\r\n  payload: [${payload}]\r\n`)
    const index = configSearchIndex(document.lines, 'EXAMPLE.COM')
    const range = configMatchRange(document, index, 40_000, 'EXAMPLE.COM')!
    const content = document.lines.join('\n')
    expect(index.count).toBe(40_000)
    expect(range.from).toBe(content.lastIndexOf('example.com'))
    expect(content.slice(range.from, range.to)).toBe('example.com')
    expect(configMatchRange(document, index, 40_001, 'EXAMPLE.COM')).toBeNull()
  })

  it('reveals literal punctuation matches and rejects an empty search', () => {
    const document = configTextDocument('header:\n  [a+b] [A+B]\n')
    const index = configSearchIndex(document.lines, '[a+b]')
    expect(configMatchRange(document, index, 2, ' [a+b] ')).toEqual({ from: 16, to: 21 })
    expect(configMatchRange(document, index, 1, '  ')).toBeNull()
  })
})
