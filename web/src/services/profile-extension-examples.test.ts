import { setLanguage } from './i18n'
import { beforeEach, afterEach, describe, expect, it } from 'vitest'
import { runInNewContext } from 'node:vm'
import { parseDocument } from 'yaml'
import { extensionExamples } from './profile-extension-examples'
import { parseSequence, sequenceEntrySummary } from './profile-sequences'

beforeEach(() => setLanguage('zh-CN'))
afterEach(() => setLanguage('system'))

describe('copyable extension examples', () => {
  it('provides valid enhancement YAML for each sequence kind with the correct deletion identity', () => {
    for (const kind of ['rules', 'proxies', 'groups'] as const) {
      const example = extensionExamples[kind]
      const model = parseSequence(example.content, kind)
      expect(model.prepend).toHaveLength(1)
      expect(model.append).toEqual([])
      expect(model.delete).toHaveLength(1)
      if (kind === 'rules') expect(model.delete[0]).toContain('RULE-SET,')
      else {
        expect(model.delete[0]).not.toContain('://')
        expect(model.prepend[0]).toMatchObject({ name: expect.any(String), type: expect.any(String) })
      }
    }
  })
  it('keeps the override example a configuration patch, rather than a sequence extension', () => {
    const doc = parseDocument(extensionExamples.override.content)
    expect(doc.errors).toEqual([])
    expect(doc.toJS()).toEqual({ 'allow-lan': true, 'unified-delay': true, profile: { 'store-selected': true } })
  })
  it('runs the script example with a plain config, prepending once and retaining unrelated fields', () => {
    const rule = 'DOMAIN-SUFFIX,example.com,DIRECT'
    const config = { rules: ['MATCH,DIRECT', rule, rule], dns: { enable: true }, proxies: [{ name: 'HK' }] }
    const result = runInNewContext(`${extensionExamples.script.content}\nmain(config, "fixture")`, { config })
    expect(result.rules).toEqual([rule, 'MATCH,DIRECT'])
    expect(result.dns).toEqual({ enable: true })
    expect(result.proxies).toEqual([{ name: 'HK' }])
  })
})

describe('node and group table summaries', () => {
  it('shows the IPv6 endpoint without credentials, UUIDs or transport URLs', () => {
    expect(sequenceEntrySummary({ name: 'Test', type: 'trojan', server: '2001:db8::1', port: 443, password: 'secret', uuid: 'private-id', 'ws-opts': { path: '/private' } }, 'proxies')).toBe('[2001:db8::1]:443')
    expect(sequenceEntrySummary({ name: 'Test', type: 'direct' }, 'proxies')).toBe('—')
  })
  it('preserves names in English group summaries', () => {
    setLanguage('en-US')
    expect(sequenceEntrySummary({ name: 'Test', type: 'select', proxies: ['规则'], use: ['我的集合'] }, 'groups')).toBe('规则, Providers: 我的集合')
  })
  it('bounds large group member summaries while retaining member/provider meaning', () => {
    expect(sequenceEntrySummary({ name: 'Test', type: 'select', proxies: ['DIRECT'], use: ['remote'] }, 'groups')).toBe('DIRECT、集合：remote')
    const members = Array.from({ length: 10000 }, (_, index) => `Node ${index}`)
    expect(sequenceEntrySummary({ name: 'Test', type: 'select', proxies: members }, 'groups')).toBe('Node 0、Node 1、Node 2 等 10000 项')
  })
})
