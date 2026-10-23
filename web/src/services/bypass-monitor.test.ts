import { describe, expect, it } from 'vitest'
import { connectionExit, logFromDevice, isContainerInterface, DeviceInventory, deviceEvidence, inSubnet, observeDevices } from './bypass-monitor'
const interfaces = [{ name: 'eth0', address: '192.168.1.20', cidr: '192.168.1.0/24' }]
describe('bypass traffic evidence', () => {
  it('only considers other devices within observed LAN subnets', () => {
    const rows = observeDevices([
      { metadata: { sourceIP: '192.168.1.20', type: 'TUN' } },
      { metadata: { sourceIP: '127.0.0.1', type: 'TUN' } },
      { metadata: { sourceIP: '198.18.0.1', type: 'TUN' } },
      { metadata: { sourceIP: '192.168.2.10', type: 'TUN' } },
      { metadata: { sourceIP: '192.168.1.100', type: 'HTTP' }, upload: 100, rule: 'Match', chains: ['DIRECT'] },
      { metadata: { sourceIP: '192.168.1.100', type: 'TUN' }, download: 200, rule: 'SrcIPCIDR', chains: ['Media', 'Node'] },
    ], interfaces)
    expect(rows).toHaveLength(1)
    expect(rows[0]).toMatchObject({ ip: '192.168.1.100', count: 2, tunCount: 1, upload: 100, download: 200, types: ['HTTP', 'TUN'] })
    expect(rows[0]?.chains).toEqual(['DIRECT', 'Media → Node'])
  })
  it('never treats explicit proxy traffic or missing inbound metadata as TUN evidence', () => {
    expect(observeDevices([{ metadata: { sourceIP: '192.168.1.100', type: 'Socks5' } }], interfaces)[0]?.tunCount).toBe(0)
    expect(observeDevices([{ metadata: { sourceIP: '192.168.1.100' } }], interfaces)[0]?.tunCount).toBe(0)
    expect(observeDevices([{ metadata: { sourceIP: '192.168.1.100', type: 'TUN' } }], [])).toEqual([])
  })
  it('rejects malformed addresses and handles prefix boundaries', () => {
    expect(inSubnet('192.168.1.256', '192.168.1.0/24')).toBe(false)
    expect(inSubnet('192.168.1.1', '192.168.1.0/invalid')).toBe(false)
    expect(inSubnet('192.168.1.1', '192.168.1.1/32')).toBe(true)
    expect(inSubnet('192.168.1.2', '192.168.1.1/32')).toBe(false)
  })
})

describe('device inventory', () => {
  it('retains discovered devices without calling them TUN devices or online', () => {
    const inventory = new DeviceInventory()
    const neighbors = [{ ip: '192.168.1.100', mac: '02:11:22:33:44:55', interface: 'eth0', state: 'STALE' }]
    const first = inventory.update([], neighbors, interfaces, 1000)[0]!
    expect(first.mac).toBe(neighbors[0]!.mac)
    expect(deviceEvidence(first, true)).toBe('已发现，暂无流量证据')
    const history = inventory.update([], [], interfaces, 2000)[0]!
    expect(history.lastObserved).toBe(1000)
    expect(deviceEvidence(history, true)).toBe('本页历史记录')
    expect(inventory.update([], [], interfaces, 1000 + 24 * 3600 * 1000 + 1)).toEqual([])
  })
  it('clears old connection counts and TUN success when connections end or become unavailable', () => {
    const inventory = new DeviceInventory()
    const active = inventory.update([{ metadata: { sourceIP: '192.168.1.100', type: 'TUN' }, upload: 500 }], [], interfaces, 1000)[0]!
    expect(deviceEvidence(active, true)).toBe('已观察到 TUN 流量')
    const ended = inventory.update([], [], interfaces, 2000)[0]!
    expect(ended).toMatchObject({ count: 0, tunCount: 0, upload: 0 })
    expect(deviceEvidence(ended, false)).toBe('历史记录，流量待验证')
    const explicit = inventory.update([{ metadata: { sourceIP: '192.168.1.100', type: 'Socks5' } }], [], interfaces, 3000)[0]!
    expect(deviceEvidence(explicit, true)).toBe('仅观察到显式代理')
  })
  it('updates MAC identity on address reuse and bounds session history', () => {
    const inventory = new DeviceInventory()
    inventory.update([], [{ ip: '192.168.1.100', mac: '02:11:22:33:44:55', interface: 'eth0', state: 'STALE' }], interfaces, 1000)
    const row = inventory.update([], [{ ip: '192.168.1.100', mac: '02:11:22:33:44:66', interface: 'eth0', state: 'REACHABLE' }], interfaces, 2000)[0]!
    expect(row.mac).toBe('02:11:22:33:44:66')
    const largeLAN = [{ name: 'eth0', address: '10.1.0.1', cidr: '10.1.0.0/16' }]
    const items = Array.from({ length: 300 }, (_, n) => ({ metadata: { sourceIP: `10.1.${Math.floor(n / 250)}.${n % 250 + 2}`, type: 'TUN' } }))
    expect(inventory.update(items, [], largeLAN, 3000)).toHaveLength(256)
  })
})

describe('diagnostic evidence', () => {
  it('only attributes known source endpoints to a device, not destination or IP prefixes', () => {
    expect(logFromDevice('[TCP] 192.168.1.5:4567 --> example.com:443 match Match', '192.168.1.5')).toBe(true)
    expect(logFromDevice('[DNS] hijack udp:198.18.0.2:53 from 192.168.1.5:1234', '192.168.1.5')).toBe(true)
    expect(logFromDevice('[TCP] 192.168.1.50:4567 --> example.com:443', '192.168.1.5')).toBe(false)
    expect(logFromDevice('[TCP] 192.168.1.6:1234 --> 192.168.1.5:443', '192.168.1.5')).toBe(false)
    expect(logFromDevice('DNS error for 192.168.1.5', '192.168.1.5')).toBe(false)
  })
  it('uses the terminal outbound, keeps unknown exits pending, and separates reject from proxy', () => {
    expect(connectionExit({ chains: ['DIRECT', 'Global'] })).toBe('直连')
    expect(connectionExit({ chains: ['REJECT', 'Policy'] })).toBe('规则拒绝')
    expect(connectionExit({ chains: ['Hong Kong', 'Proxy', 'Global'] })).toBe('代理出口')
    expect(connectionExit({})).toBe('出口待确认')
  })
})


describe('container network scope', () => {
  it('separates Docker bridges without hiding normal LAN bridges or private ranges', () => {
    expect(isContainerInterface('docker0')).toBe(true)
    expect(isContainerInterface('br-0430179e2c1a')).toBe(true)
    for (const name of ['br0', 'br-lan', 'bond0', 'eth0', 'macvlan0']) expect(isContainerInterface(name)).toBe(false)
    const inventory = new DeviceInventory()
    const interfaces = [{ name: 'docker0', address: '172.17.0.1', cidr: '172.17.0.0/16' }, { name: 'br0', address: '172.20.0.1', cidr: '172.20.0.0/24' }]
    const rows = inventory.update(['172.17.0.3', '172.20.0.50'].map(sourceIP => ({ metadata: { sourceIP, type: 'TUN' } })), [], interfaces)
    expect(rows.filter(row => !isContainerInterface(row.interface)).map(row => row.ip)).toEqual(['172.20.0.50'])
    expect(rows.find(row => row.ip === '172.17.0.3')?.interface).toBe('docker0')
    const history = inventory.update([], [], interfaces)
    expect(history.find(row => row.ip === '172.17.0.3')?.interface).toBe('docker0')
  })
})

describe('recent device traffic', () => {
  const connection = (id: string, upload = 0, download = 0, start?: string) => ({
    id, upload, download, start, metadata: { sourceIP: '192.168.1.100', type: 'TUN' },
  })
  it('ignores historical totals, expires idle residual connections, and recovers on growth', () => {
    const inventory = new DeviceInventory()
    expect(inventory.update([connection('a', 500)], [], interfaces, 0)[0]?.recentTraffic).toBe(false)
    expect(inventory.update([connection('a', 600)], [], interfaces, 10_000)[0]?.recentTraffic).toBe(true)
    for (const now of [20_000, 30_000, 40_000, 50_000, 60_000]) {
      expect(inventory.update([connection('a', 600)], [], interfaces, now)[0]?.recentTraffic).toBe(true)
    }
    expect(inventory.update([connection('a', 600)], [], interfaces, 70_000)[0]).toMatchObject({ count: 1, recentTraffic: false })
    expect(inventory.update([connection('a', 600, 1)], [], interfaces, 80_000)[0]?.recentTraffic).toBe(true)
  })
  it('detects growth per connection even when total traffic falls as another connection closes', () => {
    const inventory = new DeviceInventory()
    inventory.update([connection('large', 10000), connection('small', 1)], [], interfaces, 0)
    expect(inventory.update([connection('small', 2)], [], interfaces, 10_000)[0]?.recentTraffic).toBe(true)
  })
  it('only counts new connections with bytes and a start within the observed interval', () => {
    const inventory = new DeviceInventory()
    inventory.update([], [], interfaces, 100_000)
    expect(inventory.update([connection('old', 500, 0, new Date(0).toISOString()), connection('empty', 0, 0, new Date(105_000).toISOString())], [], interfaces, 110_000)[0]?.recentTraffic).toBe(false)
    expect(inventory.update([connection('new', 1, 0, new Date(115_000).toISOString())], [], interfaces, 120_000)[0]?.recentTraffic).toBe(true)
  })
  it('rebaselines after failed samples, long gaps, and counter resets', () => {
    const inventory = new DeviceInventory()
    inventory.update([connection('a', 500)], [], interfaces, 0)
    expect(inventory.update([connection('a', 1)], [], interfaces, 10_000)[0]?.recentTraffic).toBe(false)
    inventory.update([], [], interfaces, 20_000, false)
    expect(inventory.update([connection('a', 600)], [], interfaces, 30_000)[0]?.recentTraffic).toBe(false)
    expect(inventory.update([connection('a', 700)], [], interfaces, 100_000)[0]?.recentTraffic).toBe(false)
    expect(inventory.update([connection('a', 701)], [], interfaces, 110_000)[0]?.recentTraffic).toBe(true)
    expect(inventory.update([], [], interfaces, 170_000)[0]?.recentTraffic).toBe(false)
  })
})
