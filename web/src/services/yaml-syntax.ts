import { parser } from '@lezer/yaml'
import { highlightTree, tagHighlighter, tags } from '@lezer/highlight'
import { yamlScalarClass, yamlTokenClasses, type YamlTokenIndex } from './yaml-tokens'

export const yamlHighlightSpecs = [
  { tag: tags.propertyName, class: 'yaml-key' },
  { tag: [tags.string, tags.content, tags.attributeValue], class: 'yaml-string' },
  { tag: tags.number, class: 'yaml-number' },
  { tag: [tags.bool, tags.null], class: 'yaml-bool' },
  { tag: tags.comment, class: 'yaml-comment' },
  { tag: [tags.punctuation, tags.separator, tags.brace, tags.squareBracket], class: 'yaml-punctuation' },
  { tag: [tags.labelName, tags.typeName, tags.keyword, tags.meta], class: 'yaml-anchor' },
]

export function yamlTokenIndex(content: string): YamlTokenIndex {
  const ranges: number[] = [], lineOffsets = [0]
  for (let i = 0; i < content.length; i++) if (content[i] === '\n') lineOffsets.push(i + 1)
  highlightTree(parser.parse(content), tagHighlighter(yamlHighlightSpecs), (from, to, className) => {
    if (className === 'yaml-string') className = yamlScalarClass(content.slice(from, to)) || className
    ranges.push(from, to, yamlTokenClasses.indexOf(className))
  })
  return { ranges: new Uint32Array(ranges), lineOffsets }
}
