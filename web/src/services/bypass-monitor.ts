export interface LanInterface { name: string; address: string; cidr: string }
export interface MonitorConfig {
  tun: boolean | null; autoRoute: boolean | null; autoRedirect: boolean | null
  dnsEnabled: boolean | null; dnsListen: string; dnsHijack: string[]; routeExclude: string[]
}
export interface MonitorHost {
  interfaces: LanInterface[]; config: MonitorConfig | null; configError?: string
  forwarding: boolean | null; firewall: { readable: boolean; forwardChains?: number; natRules?: number; dropRules?: number }
  legacyJournal: boolean
}
export interface ObservedConnection {
  upload?: number; download?: number; rule?: string; rulePayload?: string; chains?: string[]
  metadata?: { sourceIP?: string; type?: string; network?: string }
}
export interface DeviceObservation {
  ip: string; count: number; tunCount: number; upload: number; download: number; types: string[]; rules: string[]; chains: string[]
}
function ipv4(value: string): number | null {
  const parts = value.split('.')
  if (parts.length !== 4 || parts.some(p => !/^\d{1,3}$/.test(p) || Number(p) > 255)) return null
  return parts.reduce((n, p) => ((n << 8) | Number(p)) >>> 0, 0)
}
export function inSubnet(address: string, cidr: string): boolean {
  const [base = '', bits = ''] = cidr.split('/')
  const ip = ipv4(address), network = ipv4(base), prefix = Number(bits)
  if (ip === null || network === null || !/^\d+$/.test(bits) || prefix < 0 || prefix > 32) return false
  const mask = prefix === 0 ? 0 : (0xffffffff << (32 - prefix)) >>> 0
  return (ip & mask) === (network & mask)
}
export function observeDevices(connections: ObservedConnection[], interfaces: LanInterface[]): DeviceObservation[] {
  const groups = new Map<string, DeviceObservation>()
  const ownAddresses = new Set(interfaces.map(i => i.address))
  for (const c of connections) {
    const ip = c.metadata?.sourceIP || ''
    if (ownAddresses.has(ip) || !interfaces.some(i => inSubnet(ip, i.cidr))) continue
    const row = groups.get(ip) || { ip, count: 0, tunCount: 0, upload: 0, download: 0, types: [], rules: [], chains: [] }
    row.count++
    const type = c.metadata?.type || ''
    if (type.toUpperCase() === 'TUN') row.tunCount++
    for (const [key, value] of [['types', type], ['rules', [c.rule, c.rulePayload].filter(Boolean).join(': ')], ['chains', (c.chains || []).join(' → ')]] as const) {
      if (value && !row[key].includes(value)) row[key].push(value)
    }
    row.upload += Number.isFinite(c.upload) ? Math.max(0, c.upload!) : 0
    row.download += Number.isFinite(c.download) ? Math.max(0, c.download!) : 0
    groups.set(ip, row)
  }
  return [...groups.values()].sort((a, b) => b.tunCount - a.tunCount || b.count - a.count || a.ip.localeCompare(b.ip))
}
