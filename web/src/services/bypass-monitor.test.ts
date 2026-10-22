import { describe, expect, it } from 'vitest'
import { inSubnet, observeDevices } from './bypass-monitor'
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
