// Docker's standard bridge names; never classify by private IP range or the
// generic "br" prefix (fnOS LAN bridges may be br0 / br-lan).
export function isContainerInterface(name: string): boolean {
  return /^docker\d+$/.test(name) || /^br-[0-9a-f]{12}$/i.test(name)
}
export interface LanInterface { name: string; address: string; cidr: string }
export interface MonitorConfig {
  tun: boolean | null; autoRoute: boolean | null; autoRedirect: boolean | null
  dnsEnabled: boolean | null; dnsListen: string; dnsHijack: string[]; routeExclude: string[]
}
export interface MonitorHost {
  interfaces: LanInterface[]; config: MonitorConfig | null; configError?: string
  forwarding: boolean | null; firewall: { readable: boolean; forwardChains?: number; natRules?: number; dropRules?: number }
  legacyJournal: boolean
  neighbors?: NeighborObservation[]; neighborStatus?: 'available' | 'unavailable' | 'unsupported'
}
export interface ObservedConnection {
  id?: string; start?: string; upload?: number; download?: number; rule?: string; rulePayload?: string; chains?: string[]
  metadata?: { sourceIP?: string; type?: string; network?: string; sourcePort?: string | number; host?: string; destinationIP?: string; destinationPort?: string | number }
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

export interface NeighborObservation { ip: string; mac: string; interface: string; state: string }
export interface InventoryDevice extends DeviceObservation {
  mac: string; interface: string; neighborState: string; discovered: boolean; lastObserved: number
}
// Session memory only. Counts and evidence always describe the current sample;
// history keeps identities visible, never stale TUN success or traffic values.
export class DeviceInventory {
  private entries = new Map<string, InventoryDevice>()
  update(connections: ObservedConnection[], neighbors: NeighborObservation[], interfaces: LanInterface[], now = Date.now()): InventoryDevice[] {
    for (const [ip, row] of this.entries) {
      if (now - row.lastObserved > 24 * 60 * 60 * 1000) this.entries.delete(ip)
      else this.entries.set(ip, { ...row, count: 0, tunCount: 0, upload: 0, download: 0, types: [], rules: [], chains: [], discovered: false, neighborState: '' })
    }
    const blank = (ip: string): InventoryDevice => ({ ip, mac: '', interface: '', neighborState: '', discovered: false, lastObserved: now, count: 0, tunCount: 0, upload: 0, download: 0, types: [], rules: [], chains: [] })
    const own = new Set(interfaces.map(i => i.address))
    for (const n of neighbors) {
      if (own.has(n.ip) || !interfaces.some(i => i.name === n.interface && inSubnet(n.ip, i.cidr))) continue
      const previous = this.entries.get(n.ip)
      const row = previous && (!previous.mac || previous.mac === n.mac) ? previous : blank(n.ip)
      this.entries.set(n.ip, { ...row, mac: n.mac, interface: n.interface, neighborState: n.state, discovered: true, lastObserved: now })
    }
    for (const traffic of observeDevices(connections, interfaces)) {
      const row = this.entries.get(traffic.ip) || blank(traffic.ip)
      // Neighbor interface is authoritative. For connection-only rows, infer
      // the most specific matching interface so container traffic stays scoped.
      const inferred = [...interfaces].filter(i => inSubnet(traffic.ip, i.cidr)).sort((a, b) => Number(b.cidr.split('/')[1]) - Number(a.cidr.split('/')[1]))[0]
      this.entries.set(traffic.ip, { ...row, ...traffic, interface: row.interface || inferred?.name || '', lastObserved: now })
    }
    const sorted = [...this.entries.values()].sort((a, b) => b.tunCount - a.tunCount || b.count - a.count || b.lastObserved - a.lastObserved || a.ip.localeCompare(b.ip)).slice(0, 256)
    this.entries = new Map(sorted.map(row => [row.ip, row]))
    return sorted
  }
}
export function deviceEvidence(device: InventoryDevice, trafficAvailable: boolean): string {
  if (!trafficAvailable) return device.discovered ? '已发现，流量待验证' : '历史记录，流量待验证'
  if (device.tunCount > 0) return '已观察到 TUN 流量'
  if (device.types.some(type => /^(http|https|socks|socks4|socks5)$/i.test(type))) return '仅观察到显式代理'
  if (device.count > 0) return '有连接，入站类型待确认'
  return device.discovered ? '已发现，暂无流量证据' : '本页历史记录'
}
// Survives navigating between pages, but resets on a browser reload.
export const bypassInventory = new DeviceInventory()

// Mihomo returns chains from the concrete outbound toward the outer groups.
// Only an explicit terminal DIRECT / REJECT establishes those outcomes.
export function connectionExit(connection: ObservedConnection): string {
  const terminal = connection.chains?.[0]
  if (!terminal) return '出口待确认'
  if (terminal.toUpperCase() === 'DIRECT') return '直连'
  if (/^REJECT(?:-DROP)?$/i.test(terminal)) return '规则拒绝'
  return '代理出口'
}
// Match the source endpoint, not a destination address or an IP prefix collision.
// Unstructured messages without a known source marker remain in global logs.
export function logFromDevice(message: string, ip: string): boolean {
  if (ipv4(ip) === null) return false
  const escaped = ip.replace(/\./g, '\\.')
  return new RegExp(`(?:^|\\s|\\])${escaped}:\\d+(?=\\s+-->|\\s+->)`).test(message)
    || new RegExp(`\\bfrom ${escaped}:\\d+(?=\\s|$)`).test(message)
}
