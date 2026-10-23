export type SettingsSearchEntry = {
  section: string
  tab: string
  label: string
  selector: string
  keywords: string
  path?: string
}

// Match names and configuration keys, never current values or credentials.
export function searchSettings<T extends SettingsSearchEntry>(entries: T[], query: string, translate: (text: string) => string): T[] {
  const normalize = (value: string) => value.toLowerCase().replace(/[\s_\-]+/g, '')
  const terms = query.trim().split(/\s+/).filter(Boolean).map(normalize).filter(Boolean)
  if (!terms.length) return []
  return entries.map((entry, index) => {
    const name = normalize(`${entry.label} ${translate(entry.label)}`)
    const text = normalize(`${name} ${entry.keywords} ${translate(entry.keywords)} ${entry.path || ''}`)
    return { entry, index, score: terms.every(term => text.includes(term)) ? (terms.every(term => name.includes(term)) ? 2 : 1) : 0 }
  }).filter(item => item.score > 0).sort((a, b) => b.score - a.score || a.index - b.index).map(item => item.entry)
}
