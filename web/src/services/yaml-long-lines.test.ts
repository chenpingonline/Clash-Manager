import { EditorState } from '@codemirror/state'
import { describe, expect, it } from 'vitest'
import { LONG_LINE_LIMIT, LONG_LINE_PREFIX, hiddenLineSize, longLines, revealLongLine, toggleLongLine } from './yaml-long-lines'

function state(content: string) { return EditorState.create({ doc: content, extensions: [longLines] }) }
function ranges(value: EditorState) {
  const result: { from: number; to: number }[] = []
  value.field(longLines).decorations.between(0, value.doc.length, (from, to) => { result.push({ from, to }) })
  return result
}

describe('long YAML line display', () => {
  const long = 'payload: [' + Array.from({ length: 40_000 }, (_, i) => `+.rule${i}.example.com`).join(', ') + ']'
  it('hides the long suffix without changing the document or its line count', () => {
    const source = `rules:\n${long}\nend: true\n`, value = state(source)
    expect(value.doc.toString()).toBe(source)
    expect(value.doc.lines).toBe(4)
    expect(ranges(value)).toEqual([{ from: 7 + LONG_LINE_PREFIX, to: 7 + long.length }])
    expect(ranges(state('x'.repeat(LONG_LINE_LIMIT)))).toEqual([])
  })
  it('expands and collapses as display effects without touching the original text', () => {
    const folded = state(long)
    const expanded = folded.update({ effects: toggleLongLine.of({ from: 0, expanded: true }) }).state
    expect(ranges(expanded)).toEqual([{ from: LONG_LINE_PREFIX, to: LONG_LINE_PREFIX }])
    const collapsed = expanded.update({ effects: toggleLongLine.of({ from: 0, expanded: false }) }).state
    expect(ranges(collapsed)).toEqual(ranges(folded))
    expect(collapsed.doc.toString()).toBe(long)
  })
  it('reveals a hidden search match and keyboard cursor before editing', () => {
    const folded = state(long), from = long.indexOf('rule39999')
    expect(revealLongLine(folded, 0, 5)).toEqual([])
    const expanded = folded.update({ effects: revealLongLine(folded, from, from + 9) }).state
    expect(expanded.field(longLines).expanded.has(0)).toBe(true)
    const cursor = folded.update({ selection: { anchor: from } }).state
    expect(cursor.field(longLines).expanded.has(0)).toBe(true)
    const edited = cursor.update({ changes: { from, to: from + 9, insert: 'last-rule' } }).state
    expect(edited.doc.toString()).toBe(long.slice(0, from) + 'last-rule' + long.slice(from + 9))
    expect(edited.field(longLines).expanded.has(0)).toBe(true)
  })
  it('maps expanded lines through insertions and clears them on full replacement', () => {
    const expanded = state(long).update({ effects: toggleLongLine.of({ from: 0, expanded: true }) }).state
    const shifted = expanded.update({ changes: { from: 0, insert: '# comment\n' } }).state
    expect(shifted.field(longLines).expanded.has(10)).toBe(true)
    const replaced = shifted.update({ changes: { from: 0, to: shifted.doc.length, insert: long } }).state
    expect(replaced.field(longLines).expanded.size).toBe(0)
    expect(ranges(replaced)).toEqual([{ from: LONG_LINE_PREFIX, to: long.length }])
  })
  it('preserves hidden suffixes when editing visible text and avoids splitting emoji', () => {
    const folded = state(long)
    const edited = folded.update({ changes: { from: 0, to: 7, insert: 'domains' } }).state
    expect(edited.doc.toString()).toBe('domains' + long.slice(7))
    const unicode = 'x'.repeat(LONG_LINE_PREFIX - 1) + '😀' + '中'.repeat(LONG_LINE_LIMIT)
    expect(ranges(state(unicode))[0]!.from).toBe(LONG_LINE_PREFIX - 1)
    expect(hiddenLineSize(new TextEncoder().encode(unicode.slice(LONG_LINE_PREFIX - 1)).byteLength)).toBe('5.9 KB')
  })
})
