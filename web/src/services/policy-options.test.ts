import { describe, expect, it } from 'vitest'
import { filterPolicyOptions } from './policy-options'

describe('listener target search and type filters', () => {
  const options = ['DIRECT', 'REJECT', 'HK group', 'HK node', '日本节点']
  const kinds = { DIRECT: 'builtin', REJECT: 'builtin', 'HK group': 'group', 'HK node': 'node', 日本节点: 'node' } as const
  it('combines a case-insensitive query with the actual target kind', () => {
    expect(filterPolicyOptions(options, kinds, 'group', '  hk ')).toEqual(['HK group'])
    expect(filterPolicyOptions(options, kinds, 'node', 'HK')).toEqual(['HK node'])
    expect(filterPolicyOptions(options, kinds, 'node', '日本')).toEqual(['日本节点'])
    expect(filterPolicyOptions(options, kinds, 'builtin', '')).toEqual(['DIRECT', 'REJECT'])
    expect(filterPolicyOptions(options, kinds, 'group', 'node')).toEqual([])
  })
  it('searches the whole subscription before any visible-list limit', () => {
    const many = Array.from({ length: 250 }, (_, i) => `node-${i}`)
    expect(filterPolicyOptions(many, undefined, 'all', 'node-249')).toEqual(['node-249'])
    expect(filterPolicyOptions(options, kinds, 'all', '')).toEqual(options)
  })
})
