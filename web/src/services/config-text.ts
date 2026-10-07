import type { YamlTokenIndex } from './yaml-tokens'
export interface ConfigTextDocument { lines: string[]; widthLine: string; syntax?: YamlTokenIndex; lineOffsets?: number[] }
export interface ConfigSearchIndex { prefix: number[]; count: number }

export function configTextDocument(content: string): ConfigTextDocument {
  const lines = content.split(/\r?\n/)
  let widthLine = '', width = 0
  for (const line of lines) {
    // Select a representative wide line; the browser measures its real glyphs.
    // Short lines cannot beat the current width, even with wide glyphs.
    if (line.length * 2 <= width && !line.includes('\t')) continue
    const columns = /[\t\x80-\uffff]/.test(line)
      ? line.replace(/\t/g, '    ').replace(/[^\x00-\x7f]/g, '  ').length
      : line.length
    if (columns > width) { width = columns; widthLine = line }
  }
  let offset = 0
  const lineOffsets = lines.map(line => { const start = offset; offset += line.length + 1; return start })
  return { lines, widthLine, lineOffsets }
}

export function configSearchIndex(lines: readonly string[], query: string): ConfigSearchIndex {
  const needle = query.trim()
  if (!needle) return { prefix: Array<number>(lines.length + 1).fill(0), count: 0 }
  const pattern = new RegExp(needle.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi')
  const prefix = [0]
  let count = 0
  for (const line of lines) {
    pattern.lastIndex = 0
    while (pattern.exec(line)) count++
    prefix.push(count)
  }
  return { prefix, count }
}

// Matches are one-based; binary search also handles intervening empty lines.
export function configMatchLine(index: ConfigSearchIndex, match: number): number {
  if (match < 1 || match > index.count) return -1
  let low = 0, high = index.prefix.length - 2
  while (low < high) {
    const middle = Math.floor((low + high) / 2)
    if (index.prefix[middle + 1]! < match) low = middle + 1
    else high = middle
  }
  return low
}

export function configMatchRange(document: ConfigTextDocument, index: ConfigSearchIndex, match: number, query: string) {
  const line = configMatchLine(index, match), needle = query.trim()
  if (line < 0 || !needle) return null
  const pattern = new RegExp(needle.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi')
  let remaining = match - index.prefix[line]!
  for (const found of document.lines[line]!.matchAll(pattern)) {
    if (--remaining === 0) {
      const from = (document.lineOffsets?.[line] ?? document.lines.slice(0, line).reduce((sum, text) => sum + text.length + 1, 0)) + found.index!
      return { from, to: from + found[0].length }
    }
  }
  return null
}
