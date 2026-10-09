import { watch } from 'vue'
import { locale, t } from './i18n'
import { Compartment, EditorSelection, EditorState, StateEffect, StateField, type Range } from '@codemirror/state'
import { Decoration, EditorView, MatchDecorator, ViewPlugin, drawSelection, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers, type DecorationSet, type ViewUpdate } from '@codemirror/view'
import { defaultKeymap } from '@codemirror/commands'
import { HighlightStyle, bracketMatching, indentUnit, syntaxHighlighting, syntaxTree } from '@codemirror/language'
import { yaml } from '@codemirror/lang-yaml'
import { yamlHighlightSpecs } from './yaml-syntax'
import { yamlScalarClass } from './yaml-tokens'
import { longLines, longLineLanguage, revealLongLine } from './yaml-long-lines'

function scalarDecorations(editor: EditorView): DecorationSet {
  const ranges: Range<Decoration>[] = [], tree = syntaxTree(editor.state)
  for (const { from, to } of editor.visibleRanges) tree.iterate({ from, to, enter(node) {
    if (node.name !== 'Literal' || node.node.parent?.name === 'Key') return
    const className = yamlScalarClass(editor.state.doc.sliceString(node.from, node.to))
    if (className) ranges.push(Decoration.mark({ class: className }).range(node.from, node.to))
  } })
  return Decoration.set(ranges, true)
}
const scalars = ViewPlugin.fromClass(class {
  decorations: DecorationSet
  constructor(editor: EditorView) { this.decorations = scalarDecorations(editor) }
  update(update: ViewUpdate) {
    if (update.docChanged || update.viewportChanged || syntaxTree(update.state) !== syntaxTree(update.startState)) this.decorations = scalarDecorations(update.view)
  }
}, { decorations: plugin => plugin.decorations })

const editorLanguage = ViewPlugin.fromClass(class {
  private stop: () => void
  constructor(view: EditorView) {
    this.stop = watch(locale, () => view.dispatch({ effects: longLineLanguage.of() }))
  }
  destroy() { this.stop() }
})
export function yamlViewExtensions() {
  return [yaml(), lineNumbers(), highlightActiveLineGutter(), highlightActiveLine(), drawSelection(), bracketMatching(),
    indentUnit.of('  '), EditorState.tabSize.of(2), syntaxHighlighting(HighlightStyle.define(yamlHighlightSpecs)), longLines, editorLanguage, scalars]
}

const activeMatch = StateEffect.define<{ from: number; to: number } | null>()
const queryConfig = new Compartment()
const matchField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(value, transaction) {
    for (const effect of transaction.effects) if (effect.is(activeMatch)) {
      const range = effect.value
      return range ? Decoration.set([Decoration.mark({ class: 'config-search-match current' }).range(range.from, range.to)]) : Decoration.none
    }
    return value.map(transaction.changes)
  },
  provide: field => EditorView.decorations.from(field),
})

function visibleMatches(query: string) {
  if (!query.trim()) return []
  const matcher = new MatchDecorator({
    regexp: new RegExp(query.trim().replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi'),
    decoration: Decoration.mark({ class: 'config-search-match' }),
  })
  return ViewPlugin.fromClass(class {
    decorations: DecorationSet
    constructor(view: EditorView) { this.decorations = matcher.createDeco(view) }
    update(update: ViewUpdate) { this.decorations = matcher.updateDeco(update, this.decorations) }
  }, { decorations: plugin => plugin.decorations })
}

export function previewState(content: string, query: string) {
  return EditorState.create({ doc: content, extensions: [
    ...yamlViewExtensions(), EditorState.readOnly.of(true), EditorView.editable.of(false),
    EditorView.contentAttributes.of(() => ({ 'aria-label': t('配置内容'), tabindex: '0' })),
    keymap.of(defaultKeymap), queryConfig.of(visibleMatches(query)), matchField,
  ] })
}
export function createConfigViewer(parent: HTMLElement, content: string, query: string) {
  return new EditorView({ parent, state: previewState(content, query) })
}
export function setConfigQuery(view: EditorView, query: string) {
  view.dispatch({ effects: [queryConfig.reconfigure(visibleMatches(query)), activeMatch.of(null)] })
}
export function revealConfigMatch(view: EditorView, range: { from: number; to: number } | null) {
  view.dispatch({ effects: range ? [...revealLongLine(view.state, range.from, range.to), activeMatch.of(range), EditorView.scrollIntoView(EditorSelection.range(range.from, range.to), { x: 'center', y: 'center' })] : activeMatch.of(null) })
}
