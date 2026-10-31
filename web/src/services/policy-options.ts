export type PolicyKind = 'group' | 'node' | 'builtin'
export type PolicyFilter = 'all' | PolicyKind

export function filterPolicyOptions(options: string[], kinds: Record<string, PolicyKind> | undefined, filter: PolicyFilter, search: string): string[] {
  const term = search.trim().toLocaleLowerCase()
  return options.filter(name => (filter === 'all' || kinds?.[name] === filter) && (!term || name.toLocaleLowerCase().includes(term)))
}
