import type { IndependentListener } from '@/types/api'

export function listenerID() {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
}
export function availableListenerPort(entries: IndependentListener[], reserved: number[], start = 1080): number | null {
  const used = new Set([...reserved, ...entries.map(entry => entry.port)])
  for (let port = Math.max(1, start); port <= 65535; port++) if (!used.has(port)) return port
  return null
}
export function availableListenerName(name: string, entries: IndependentListener[]) {
  const used = new Set(entries.map(entry => entry.name))
  if (!used.has(name)) return name
  for (let suffix = 2; ; suffix++) {
    const candidate = `${name.slice(0, 90)} (${suffix})`
    if (!used.has(candidate)) return candidate
  }
}
export function copyListener(entry: IndependentListener): IndependentListener {
  return { ...entry, udp: entry.type !== 'http' && entry.udp !== false, users: (entry.users || []).map(user => ({ ...user })) }
}
