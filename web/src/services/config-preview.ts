import { isAlias, isMap, isScalar, isSeq, parseDocument, visit } from 'yaml'

export type ConfigFormat = 'original' | 'formatted' | 'compact'

// Strict entry point for explicit draft changes. Invalid YAML is never replaced.
export function formatConfigYaml(content: string, format: Exclude<ConfigFormat, 'original'>): string {
  const document = parseDocument(content, { intAsBigInt: true })
  if (document.errors.length) throw new Error(`YAML 语法错误：${document.errors[0]!.message}`)
  if (!isMap(document.contents)) throw new Error('配置必须是 YAML 对象')
  visit(document, {
    Collection(_key, node, path) {
      // Keep top-level sections and their object lists easy to scan, while
      // displaying each proxy/group/provider and scalar array inline.
      node.flow = format === 'compact' && (path.length > 3 || (isSeq(node) && node.items.every(item => isScalar(item) || isAlias(item))))
    },
  })
  return document.toString({ lineWidth: 0, indent: 2 })
}

// Presentation only: callers retain the original text for editing/copying.
export function configPreview(content: string, format: ConfigFormat): string {
  if (format === 'original' || !content.trim()) return content
  try { return formatConfigYaml(content, format) } catch { return content }
}

export function compactConfigPreview(content: string): string {
  return configPreview(content, 'compact')
}
