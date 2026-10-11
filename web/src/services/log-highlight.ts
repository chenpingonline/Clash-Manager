import { highlightParts } from './logs'

export type LogTokenKind = 'protocol' | 'address' | 'rule' | 'policy' | 'error' | 'warn' | 'level' | ''
export interface LogMessagePart { text: string; kind: LogTokenKind; match: boolean }
interface Range { start: number; end: number; kind: LogTokenKind }

// Presentation only: preserve every character and keep log output as escaped
// text. Search ranges are computed on the original message, even across tokens.
export function logMessageParts(text: string, query = ''): LogMessagePart[] {
  const ranges: Range[] = []
  const add = (pattern: RegExp, kind: LogTokenKind, group = 0) => {
    for (const match of text.matchAll(pattern)) {
      const value = match[group]!
      const start = match.index! + (group ? match[0].length - value.length : 0)
      ranges.push({ start, end: start + value.length, kind })
    }
  }
  add(/\[(?:TCP|UDP|ICMP|HTTP|HTTPS|SOCKS5?)\]/gi, 'protocol')
  add(/\b(?:\d{1,3}\.){3}\d{1,3}(?::\d{1,5})?\b|\[[\da-f:]+(?:%[\w.-]+)?\](?::\d{1,5})?/gi, 'address')
  add(/\b(?:[a-z\d](?:[a-z\d-]*[a-z\d])?\.)+[a-z]{2,}(?::\d{1,5})?\b/gi, 'address')
  add(/\bmatch\s+([\w-]+(?:\([^\r\n)]*\))?)/gi, 'rule', 1)
  add(/\busing\s+([^\r\n]+)/gi, 'policy', 1)
  add(/\b(?:error|fatal|panic|failed|failure|timeout|timed out|refused|unreachable|cannot assign requested address)\b/gi, 'error')
  add(/\blevel=(?:trace|debug|info|warn|warning|error|fatal|panic)\b/gi, 'level')
  add(/\blevel=(?:warn|warning)\b/gi, 'warn')
  add(/\blevel=(?:error|fatal|panic)\b/gi, 'error')

  const search: Array<{ start: number; end: number }> = []
  let offset = 0
  for (const part of highlightParts(text, query)) {
    if (part.match) search.push({ start: offset, end: offset + part.text.length })
    offset += part.text.length
  }
  const boundaries = [...new Set([0, text.length, ...ranges.flatMap(r => [r.start, r.end]), ...search.flatMap(r => [r.start, r.end])])].sort((a, b) => a - b)
  const starts = new Map<number, number[]>(), ends = new Map<number, number[]>()
  ranges.forEach((range, index) => {
    starts.set(range.start, [...(starts.get(range.start) || []), index])
    ends.set(range.end, [...(ends.get(range.end) || []), index])
  })
  const active = new Set<number>()
  const parts: LogMessagePart[] = []
  let searchIndex = 0
  for (let i = 0; i < boundaries.length - 1; i++) {
    const start = boundaries[i]!, end = boundaries[i + 1]!
    for (const index of ends.get(start) || []) active.delete(index)
    for (const index of starts.get(start) || []) active.add(index)
    // Later, more specific tokens take precedence over a broad policy range.
    let priority = -1
    for (const index of active) priority = Math.max(priority, index)
    const kind = priority >= 0 ? ranges[priority]!.kind : ''
    while (search[searchIndex] && search[searchIndex]!.end <= start) searchIndex++
    const match = Boolean(search[searchIndex] && search[searchIndex]!.start <= start && search[searchIndex]!.end >= end)
    const previous = parts[parts.length - 1]
    if (previous && previous.kind === kind && previous.match === match) previous.text += text.slice(start, end)
    else parts.push({ text: text.slice(start, end), kind, match })
  }
  return parts
}
