import { describe, expect, it } from 'vitest'
import { searchSettings, type SettingsSearchEntry } from './settings-search'
const entries: SettingsSearchEntry[] = [
  { section: 'dns', tab: 'basic', label: 'DNS 监听地址', selector: '#dns-listen', keywords: 'listen', path: 'DNS 与解析 › 基础设置' },
  { section: 'dns', tab: 'fake-ip', label: 'Fake IP 过滤', selector: '#dns-filter', keywords: 'fake-ip-filter', path: 'DNS 与解析 › 域名策略' },
  { section: 'network', tab: '', label: '监听地址', selector: '#bind-address', keywords: 'bind-address allow-lan', path: '网络与端口' },
]
const t = (text: string) => ({ 'DNS 监听地址': 'DNS listen address', '监听地址': 'Bind address' })[text] || text

describe('settings search', () => {
  it('matches Chinese labels and translated English labels', () => {
    expect(searchSettings(entries, '监听', t)).toHaveLength(2)
    expect(searchSettings(entries, 'listen address', t).map(item => item.selector)).toEqual(['#dns-listen'])
  })
  it('normalizes case, hyphens and underscores in configuration keys', () => {
    expect(searchSettings(entries, 'FAKE_IP', t)[0]?.selector).toBe('#dns-filter')
    expect(searchSettings(entries, 'allow-lan', t)[0]?.selector).toBe('#bind-address')
  })
  it('requires every word, including the category context', () => {
    expect(searchSettings(entries, 'DNS 监听', t).map(item => item.selector)).toEqual(['#dns-listen'])
    expect(searchSettings(entries, 'DNS 不存在', t)).toEqual([])
  })
  it('ranks direct label matches before category matches', () => {
    expect(searchSettings([...entries].reverse(), 'DNS', t)[0]?.selector).toBe('#dns-listen')
  })
  it('returns no results for empty, punctuation-only or unknown queries', () => {
    for (const query of ['', '  ', '---', 'missing']) expect(searchSettings(entries, query, t)).toEqual([])
  })
})
