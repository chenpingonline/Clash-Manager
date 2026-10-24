import type { SequenceExtension } from './profile-sequences'

export interface InstalledApp { id: string; name: string; rule: string; reason?: string }

// Move selected app rules to the front and remove duplicate custom entries.
// Keep unrelated exclusions, additions and advanced fields intact.
export function addAppDirectRules(model: SequenceExtension, apps: InstalledApp[]): SequenceExtension {
  const rules = [...new Set(apps.map(app => app.rule).filter(rule => rule.startsWith('PROCESS-PATH-REGEX,') && rule.endsWith(',DIRECT')))]
  if (!rules.length) return model
  const selected = new Set(rules)
  return {
    ...model,
    prepend: [...rules, ...model.prepend.filter(rule => typeof rule !== 'string' || !selected.has(rule))],
    append: model.append.filter(rule => typeof rule !== 'string' || !selected.has(rule)),
    delete: model.delete.filter(rule => !selected.has(rule)),
    'find-process-mode': 'always',
  }
}
