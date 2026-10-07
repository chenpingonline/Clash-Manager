import { EditorSelection, MapMode, StateEffect, StateField, type EditorState, type Range, type Text } from '@codemirror/state'
import { Decoration, EditorView, WidgetType, type DecorationSet } from '@codemirror/view'

export const LONG_LINE_LIMIT = 2_000
export const LONG_LINE_PREFIX = 120
export const toggleLongLine = StateEffect.define<{ from: number; expanded: boolean }>()

function prefixLength(text: string) {
  // Never split an emoji's UTF-16 surrogate pair at the display boundary.
  const previous = text.charCodeAt(LONG_LINE_PREFIX - 1)
  return previous >= 0xd800 && previous <= 0xdbff ? LONG_LINE_PREFIX - 1 : LONG_LINE_PREFIX
}
export function hiddenLineSize(bytes: number) {
  return bytes < 1024 ? `${bytes} B` : bytes < 1024 * 1024 ? `${(bytes / 1024).toFixed(1)} KB` : `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

class LongLineToggle extends WidgetType {
  constructor(readonly from: number, readonly bytes: number, readonly expanded: boolean) { super() }
  eq(other: LongLineToggle) { return this.from === other.from && this.bytes === other.bytes && this.expanded === other.expanded }
  toDOM(view: EditorView) {
    const button = document.createElement('button')
    button.type = 'button'
    button.className = 'yaml-long-line-toggle'
    button.textContent = this.expanded ? '收起超长行' : `展开更多（${hiddenLineSize(this.bytes)}）`
    button.setAttribute('aria-expanded', String(this.expanded))
    button.title = this.expanded ? '收起此行，仅显示开头；配置内容保持完整' : '为保持流畅，超长行暂只显示开头；点击显示此行的完整内容'
    button.addEventListener('click', event => {
      event.preventDefault(); event.stopPropagation()
      const line = view.state.doc.lineAt(this.from)
      const boundary = line.from + prefixLength(line.text)
      const head = view.state.selection.main.head
      view.dispatch({
        effects: toggleLongLine.of({ from: line.from, expanded: !this.expanded }),
        ...(this.expanded && head > boundary && head <= line.to ? { selection: EditorSelection.cursor(boundary) } : {}),
      })
    })
    return button
  }
  ignoreEvent() { return true }
}

interface LongLines { expanded: Set<number>; decorations: DecorationSet }
function decorations(doc: Text, expanded: Set<number>) {
  const ranges: Range<Decoration>[] = []
  let from = 0
  for (const text of doc.iterLines()) {
    if (text.length > LONG_LINE_LIMIT) {
      const boundary = from + prefixLength(text)
      const bytes = new TextEncoder().encode(text.slice(boundary - from)).byteLength
      const opened = expanded.has(from)
      const widget = new LongLineToggle(from, bytes, opened)
      ranges.push(opened ? Decoration.widget({ widget, side: -1 }).range(boundary)
        : Decoration.replace({ widget, inclusive: false }).range(boundary, from + text.length))
    }
    from += text.length + 1
  }
  return Decoration.set(ranges, true)
}

export const longLines = StateField.define<LongLines>({
  create(state) { return { expanded: new Set(), decorations: decorations(state.doc, new Set()) } },
  update(value, transaction) {
    const changed = transaction.docChanged
    const expanded = new Set<number>()
    for (const from of value.expanded) {
      const mapped = changed ? transaction.changes.mapPos(from, 1, MapMode.TrackAfter) : from
      if (mapped !== null && mapped <= transaction.newDoc.length) {
        const line = transaction.newDoc.lineAt(mapped)
        if (line.length > LONG_LINE_LIMIT) expanded.add(line.from)
      }
    }
    let toggled = false
    const collapsed = new Set<number>()
    for (const effect of transaction.effects) if (effect.is(toggleLongLine)) {
      const line = transaction.newDoc.lineAt(effect.value.from)
      if (line.length <= LONG_LINE_LIMIT) continue
      toggled = true
      if (effect.value.expanded) expanded.add(line.from)
      else { expanded.delete(line.from); collapsed.add(line.from) }
    }
    // Keyboard navigation into hidden text reveals it before the next edit.
    if (transaction.selection) for (const range of transaction.newSelection.ranges) for (const position of [range.anchor, range.head]) {
      const line = transaction.newDoc.lineAt(position)
      if (line.length > LONG_LINE_LIMIT && position > line.from + prefixLength(line.text) && !collapsed.has(line.from) && !expanded.has(line.from)) {
        expanded.add(line.from); toggled = true
      }
    }
    if (!changed && !toggled) return value
    return { expanded, decorations: decorations(transaction.newDoc, expanded) }
  },
  provide: field => EditorView.decorations.from(field, value => value.decorations),
})

export function revealLongLine(state: EditorState, from: number, to: number) {
  const line = state.doc.lineAt(from)
  return line.length > LONG_LINE_LIMIT && to > line.from + prefixLength(line.text)
    ? [toggleLongLine.of({ from: line.from, expanded: true })] : []
}
