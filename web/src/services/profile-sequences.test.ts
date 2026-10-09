import { describe, expect, it } from 'vitest'
import { copyGroup, effectiveEntries, emptySequence, makeRule, moveEntry, parseSequence, ruleParts, serializeSequence, sequenceRows, filterSequenceRows, setOriginalExclusions } from './profile-sequences'
import { parseProxyInput, parseProxyURI } from './proxy-uri'

describe('subscription sequence drafts', () => {
  it('roundtrips unrecognised proxy/group fields through visual updates', () => {
    expect(parseSequence('', 'groups')).toEqual(emptySequence())
    const original = parseSequence('prepend:\n - name: Custom\n   type: select\n   proxies: [DIRECT]\n   custom-option: {nested: [one, two]}\nappend: []\ndelete: []\nfuture-field: keep\n', 'groups')
    const next = { ...original, delete: ['Old'] }
    expect(parseSequence(serializeSequence(next), 'groups')).toEqual(next)
  })
  it('rejects invalid advanced lists, duplicate YAML keys and malformed entries', () => {
    for (const content of ['[]', 'prepend: nope', 'delete: [123]', 'prepend: [123]', 'prepend: []\nprepend: []']) expect(() => parseSequence(content, 'rules')).toThrow()
    expect(() => parseSequence('prepend: [{server: test}]', 'proxies')).toThrow(/name/)
  })
  it('deletes original identities, retains order, and moves using the original index when filtering', () => {
    const model = { ...emptySequence(), prepend: ['DOMAIN,a,DIRECT', 'DOMAIN,hidden,DIRECT', 'DOMAIN,c,DIRECT'], append: ['DOMAIN,z,DIRECT'], delete: ['MATCH,DIRECT'] }
    expect(effectiveEntries(['MATCH,DIRECT', 'DOMAIN,base,DIRECT'], model)).toEqual([...model.prepend, 'DOMAIN,base,DIRECT', ...model.append])
    expect(moveEntry(model, 'prepend', 2, -1).prepend).toEqual(['DOMAIN,a,DIRECT', 'DOMAIN,c,DIRECT', 'DOMAIN,hidden,DIRECT'])
    expect(model.prepend[1]).toBe('DOMAIN,hidden,DIRECT')
  })
  it('parses logical rules without splitting the operand and validates structured rule inputs', () => {
    expect(ruleParts('AND,((DOMAIN,a),(NETWORK,tcp)),DIRECT')).toEqual(['AND', '((DOMAIN,a),(NETWORK,tcp))', 'DIRECT'])
    expect(makeRule('MATCH', '', 'DIRECT')).toBe('MATCH,DIRECT')
    expect(makeRule('RULE-SET', 'test', 'Proxy', true)).toBe('RULE-SET,test,Proxy,no-resolve')
    expect(makeRule('DST-PORT', '80/443/1000-2000', 'DIRECT')).toContain('80/443/1000-2000')
    for (const [type, payload] of [['DOMAIN', 'a,b'], ['DST-PORT', '70000'], ['DST-PORT', '90-80'], ['NETWORK', 'other'], ['AND', '(bad']]) expect(() => makeRule(type!, payload!, 'DIRECT')).toThrow()
  })
})
describe('group copies and batch exclusions', () => {
  it('copies all fields, avoids occupied names and detaches nested values from the source', () => {
    const source = { name: 'Hong Kong', type: 'url-test', proxies: ['HK-1', 'HK-2'], use: ['Airport'], lazy: false, interval: 0, 'custom-option': { nested: ['one', 'two'] } }
    const copy = copyGroup(source, ['Hong Kong-Copy', 'Hong Kong-Copy-2'], 'Copy')
    expect(copy).toEqual({ ...source, name: 'Hong Kong-Copy-3' })
    ;(copy.proxies as string[]).push('DIRECT')
    ;(copy['custom-option'] as { nested: string[] }).nested[0] = 'changed'
    expect(source.proxies).toEqual(['HK-1', 'HK-2'])
    expect(source['custom-option'].nested).toEqual(['one', 'two'])
    expect(parseSequence(serializeSequence({ ...emptySequence(), prepend: [copy] }), 'groups').prepend).toEqual([copy])
  })
  it('excludes all downloaded originals once and restores them without losing custom additions or stale exclusions', () => {
    const base = Array.from({ length: 12000 }, (_, index) => `DOMAIN,site-${index}.example,DIRECT`)
    const model = { ...emptySequence(), prepend: ['DOMAIN,local.example,DIRECT'], append: ['MATCH,DIRECT'], delete: [base[0]!, 'DOMAIN,old.example,DIRECT'], 'future-option': true }
    const excluded = setOriginalExclusions(model, [...base, base[0]!], true)
    expect(excluded.delete).toHaveLength(12001)
    expect(effectiveEntries(base, excluded)).toEqual([...model.prepend, ...model.append])
    expect(setOriginalExclusions(excluded, base, true)).toEqual(excluded)
    expect(model.delete).toEqual([base[0], 'DOMAIN,old.example,DIRECT'])
    const restored = setOriginalExclusions(excluded, base, false)
    expect(restored).toEqual({ ...model, delete: ['DOMAIN,old.example,DIRECT'] })
    expect(effectiveEntries([...base, 'DOMAIN,new.example,DIRECT'], excluded)).toContain('DOMAIN,new.example,DIRECT')
    expect(setOriginalExclusions(model, [], false)).toEqual(model)
  })
})
describe('rule table filtering', () => {
  const base = ['DOMAIN,base.example,Proxy', 'RULE-SET,geosite-netflix,Netflix', 'MATCH,DIRECT']
  const model = { ...emptySequence(), prepend: ['DOMAIN,nas.example,DIRECT', 'DOMAIN,ads.example,REJECT'], append: ['DOMAIN,tail.example,DIRECT'], delete: [base[1]!, 'DOMAIN,removed-by-subscription,DIRECT'] }
  it('keeps excluded originals visible and preserves source index and global order through filters', () => {
    const rows = sequenceRows(base, model)
    expect(rows.map(row => row.side)).toEqual(['prepend', 'prepend', 'base', 'base', 'base', 'append'])
    expect(filterSequenceRows(rows, 'netflix', 'deleted')).toMatchObject([{ side: 'base', index: 1, order: 4, deleted: true }])
    expect(filterSequenceRows(rows, 'TAIL', 'append')).toMatchObject([{ index: 0, order: 6 }])
    expect(filterSequenceRows(rows, 'REJECT', 'prepend')).toMatchObject([{ index: 1, order: 2 }])
    expect(filterSequenceRows(rows, 'netflix', 'prepend')).toEqual([])
    expect(rows[3]?.item).toBe(base[1])
    expect(effectiveEntries(base, model)).not.toContain(base[1])
  })
  it('updates exclusion filters after restoring a rule without renumbering originals', () => {
    expect(filterSequenceRows(sequenceRows(base, { ...model, delete: [] }), '', 'deleted')).toEqual([])
    expect(filterSequenceRows(sequenceRows(base, model), '', 'base')).toHaveLength(3)
  })
  it('retains stable indices for reorder and delete while searching large subscriptions', () => {
    const large = Array.from({ length: 12000 }, (_, index) => `DOMAIN,site-${index}.example,DIRECT`)
    const rows = sequenceRows(large, model)
    const result = filterSequenceRows(rows, 'site-11999', 'base')
    expect(result).toMatchObject([{ index: 11999, order: 12002, deleted: false }])
    expect(rows).toHaveLength(12003)
  })
})
const b64 = (value: string) => Buffer.from(value).toString('base64')
describe('proxy URI import', () => {
  it('imports whole Base64 subscriptions with Unicode names and preserves link order', () => {
    const result = parseProxyInput(b64('trojan://test@server.test:443#%E9%A6%99%E6%B8%AF\nanytls://test@server.test:8443#AnyTLS'))
    expect(result.map(proxy => proxy.name)).toEqual(['香港', 'AnyTLS'])
    expect(result[0]?.password).toBe('test')
    expect(parseProxyURI('anytls://user:pass@server.test')).toMatchObject({ password: 'pass', udp: true })
    expect(() => parseProxyInput(b64('  '))).toThrow('为空')
  })
  it('imports modern and legacy Shadowsocks user info', () => {
    expect(parseProxyURI(`ss://${b64('aes-128-gcm:password')}@host.test:1234#SS`)).toMatchObject({ cipher: 'aes-128-gcm', password: 'password', port: 1234 })
    expect(parseProxyURI(`ss://${b64('aes-256-gcm:secret@host.test:444')}`)).toMatchObject({ cipher: 'aes-256-gcm', password: 'secret', port: 444 })
  })
  it('preserves VLESS Reality, transport, TLS and IPv6 fields', () => {
    expect(parseProxyURI('vless://uuid@[::1]:8443?security=reality&sni=tls.test&pbk=key&sid=id&fp=chrome&type=ws&path=%2Fproxy&host=host.test&allowInsecure=0#Test')).toMatchObject({ server: '::1', port: 8443, tls: true, servername: 'tls.test', 'skip-cert-verify': false, 'client-fingerprint': 'chrome', 'reality-opts': { 'public-key': 'key', 'short-id': 'id' }, 'ws-opts': { path: '/proxy', headers: { Host: 'host.test' } } })
  })
  it('preserves VMess data and validates protocols and ports without leaking credentials', () => {
    expect(parseProxyURI('vmess://' + b64(JSON.stringify({ ps: 'VM', add: 'host.test', port: 443, id: 'uuid', net: 'grpc', path: 'service', tls: 'tls', sni: 'tls.test' })))).toMatchObject({ type: 'vmess', uuid: 'uuid', tls: true, 'grpc-opts': { 'grpc-service-name': 'service' } })
    expect(() => parseProxyInput('trojan://very-secret@host.test:99999#bad')).toThrow('第 1 行')
    try { parseProxyInput('trojan://very-secret@host.test:99999#bad') } catch (cause) { expect(String(cause)).not.toContain('very-secret') }
    expect(() => parseProxyInput('trojan://a@host.test#same\ntrojan://b@host.test#same')).toThrow('名称重复')
    expect(() => parseProxyInput('unsupported://private@host.test')).toThrow()
  })
  it('maps Hysteria2, TUIC and authenticated HTTP/SOCKS fields', () => {
    expect(parseProxyURI('hy2://pass@host.test?obfs=salamander&obfs-password=obfs&insecure=1')).toMatchObject({ type: 'hysteria2', password: 'pass', obfs: 'salamander', 'obfs-password': 'obfs', 'skip-cert-verify': true })
    expect(parseProxyURI('tuic://uuid:pass@host.test?congestion-controller=bbr')).toMatchObject({ uuid: 'uuid', password: 'pass', 'congestion-controller': 'bbr' })
    expect(parseProxyURI('https://user:pass@host.test#https')).toMatchObject({ type: 'http', tls: true, username: 'user', password: 'pass', port: 443 })
    expect(parseProxyURI('socks5://user:pass@host.test:1080')).toMatchObject({ type: 'socks5', username: 'user', password: 'pass' })
  })
})
