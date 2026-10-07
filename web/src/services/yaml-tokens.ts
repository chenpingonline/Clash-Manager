import { highlightParts } from './logs'
export const yamlTokenClasses = ['', 'yaml-key', 'yaml-string', 'yaml-number', 'yaml-bool', 'yaml-comment', 'yaml-punctuation', 'yaml-anchor']
export interface YamlTokenIndex { ranges: Uint32Array; lineOffsets: number[] }

export function yamlScalarClass(text: string): string {
  if (/^(?:true|false|null|~)$/i.test(text)) return 'yaml-bool'
  if (/^[-+]?(?:0x[\da-fA-F]+|0o[0-7]+|(?:\d[\d_]*\.?[\d_]*|\.\d[\d_]*)(?:e[-+]?\d+)?|\.inf|\.nan)$/i.test(text)) return 'yaml-number'
  return ''
}

export function yamlLineParts(text: string, line: number, index?: YamlTokenIndex) {
  if (!index) return [{ text, className: '' }]
  const offset = index.lineOffsets[line] || 0, end = offset + text.length, ranges = index.ranges
  let low = 0, high = ranges.length / 3
  while (low < high) {
    const middle = Math.floor((low + high) / 2)
    if (ranges[middle * 3 + 1]! <= offset) low = middle + 1
    else high = middle
  }
  const parts: { text: string; className: string }[] = []
  let cursor = offset
  for (let i = low * 3; i < ranges.length && ranges[i]! < end; i += 3) {
    const from = Math.max(offset, ranges[i]!), to = Math.min(end, ranges[i + 1]!)
    if (from > cursor) parts.push({ text: text.slice(cursor - offset, from - offset), className: '' })
    if (to > from) parts.push({ text: text.slice(from - offset, to - offset), className: yamlTokenClasses[ranges[i + 2]!] || '' })
    cursor = to
  }
  if (cursor < end) parts.push({ text: text.slice(cursor - offset), className: '' })
  return parts
}

// Search matches may cross syntax boundaries (for example, "port: 7890").
export function yamlSearchParts(text: string, line: number, index: YamlTokenIndex | undefined, query: string) {
  const tokens = yamlLineParts(text, line, index)
  let tokenIndex = 0, tokenOffset = 0
  return highlightParts(text, query).map(part => {
    let length = part.text.length
    const tokensInPart: { text: string; className: string }[] = []
    while (length > 0 && tokenIndex < tokens.length) {
      const token = tokens[tokenIndex]!, take = Math.min(length, token.text.length - tokenOffset)
      tokensInPart.push({ text: token.text.slice(tokenOffset, tokenOffset + take), className: token.className })
      length -= take; tokenOffset += take
      if (tokenOffset === token.text.length) { tokenIndex++; tokenOffset = 0 }
    }
    return { match: part.match, tokens: tokensInPart }
  })
}
