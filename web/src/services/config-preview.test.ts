import { describe, expect, it } from 'vitest'
import { parseDocument } from 'yaml'
import { compactConfigPreview, formatConfigYaml } from './config-preview'

describe('compact YAML preview', () => {
  it('formats and compresses without changing values, while rejecting invalid draft changes', () => {
    const input = 'proxies: [{ name: HK, type: trojan, port: 443, password: "null" }]\nrules: ["MATCH,DIRECT"]\n'
    const expanded = formatConfigYaml(input, 'formatted'), compact = formatConfigYaml(expanded, 'compact')
    expect(expanded).toContain('  - name: HK\n')
    expect(compact).toContain('- { name: HK, type: trojan,')
    expect(parseDocument(compact).toJS()).toEqual(parseDocument(input).toJS())
    expect(() => formatConfigYaml('rules: [', 'compact')).toThrow('YAML 语法错误')
    expect(() => formatConfigYaml('- value', 'formatted')).toThrow('配置必须是 YAML 对象')
  })
  it('displays each proxy, group and provider inline without changing YAML values', () => {
    const source = `# configuration\nport: 7890\nsecret: "true"\nlarge: 900719925474099312345\ndns:\n  nameserver:\n    - system\n    - https://dns.example/dns-query\nproxies:\n  - name: HK\n    type: trojan\n    server: fixture.invalid\n    port: 443\n    password: "x,y:#{}[]"\n    alpn:\n      - h2\n      - http/1.1\nproxy-groups:\n  - name: 节点选择\n    type: select\n    proxies:\n      - HK\n      - DIRECT\nrule-anchor:\n  domain: &domain\n    type: inline\n    behavior: domain\nrule-providers:\n  local:\n    <<: *domain\n    payload:\n      - '+.example.com'\nrules:\n  - DOMAIN-SUFFIX,example.com,DIRECT\n  - MATCH,DIRECT\nscript: |\n  const text = "commas, brackets[ ]";\n  // keep this text\n`
    const formatted = compactConfigPreview(source)
    expect(formatted).toContain('- { name: HK, type: trojan,')
    expect(formatted).toContain('proxies: [ HK, DIRECT ]')
    expect(formatted).toContain('nameserver: [ system,')
    expect(formatted).toContain('# configuration')
    const options = { intAsBigInt: true, merge: true }
    expect(parseDocument(formatted, options).errors).toEqual([])
    expect(parseDocument(formatted, options).toJS()).toEqual(parseDocument(source, options).toJS())
    expect(formatted.split('\n').length).toBeLessThan(source.split('\n').length / 2)
  })

  it('keeps huge inline payloads on one line and preserves punctuation and comments', () => {
    const items = Array.from({ length: 20_000 }, (_, i) => `    - '+.rule${i}.example.com'`).join('\n')
    const source = `rule-providers:\n  local:\n    type: inline\n    behavior: domain\n    payload:\n${items}\n    # provider comment\nrules:\n  - DOMAIN-SUFFIX,example.com,DIRECT\n  - MATCH,DIRECT\n`
    const formatted = compactConfigPreview(source)
    expect(formatted.split('\n').length).toBeLessThan(15)
    const payloadLine = formatted.split('\n').find(line => line.includes('payload:'))!
    expect(payloadLine).toContain('+.rule0.example.com')
    expect(payloadLine).toContain('+.rule19999.example.com')
    expect(formatted).toContain('# provider comment')
    expect(parseDocument(formatted).errors).toEqual([])
    expect(parseDocument(formatted).toJS()).toEqual(parseDocument(source).toJS())
  })

  it('leaves malformed YAML, non-object documents and empty content intact', () => {
    for (const content of ['rules: [', 'a: 1\na: 2\n', '---\na: 1\n---\na: 2\n', '- value\n', '', '  ']) {
      expect(compactConfigPreview(content)).toBe(content)
    }
  })
})
