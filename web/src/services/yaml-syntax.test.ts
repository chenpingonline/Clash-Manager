import { describe, expect, it } from 'vitest'
import { yamlTokenIndex } from './yaml-syntax'
import { yamlLineParts, yamlSearchParts } from './yaml-tokens'

describe('YAML syntax colors and search', () => {
  const content = 'port: 7890\r\nenable: true\r\nsecret: "true"\r\n# enable: false\r\nproxies: [{name: HK, port: 443}]\r\nscript: |\r\n  true # text\r\n'
  const index = yamlTokenIndex(content)
  const lines = content.split(/\r?\n/)
  it('distinguishes keys, numbers, booleans, quoted strings and comments', () => {
    expect(yamlLineParts(lines[0]!, 0, index)).toContainEqual({ text: 'port', className: 'yaml-key' })
    expect(yamlLineParts(lines[0]!, 0, index)).toContainEqual({ text: '7890', className: 'yaml-number' })
    expect(yamlLineParts(lines[1]!, 1, index)).toContainEqual({ text: 'true', className: 'yaml-bool' })
    expect(yamlLineParts(lines[2]!, 2, index)).toContainEqual({ text: '"true"', className: 'yaml-string' })
    expect(yamlLineParts(lines[3]!, 3, index)).toContainEqual({ text: '# enable: false', className: 'yaml-comment' })
    expect(yamlLineParts(lines[6]!, 6, index).every(part => part.className !== 'yaml-bool' && part.className !== 'yaml-comment')).toBe(true)
  })
  it('preserves every character and colors nested flow-map keys', () => {
    lines.forEach((line, number) => expect(yamlLineParts(line, number, index).map(part => part.text).join('')).toBe(line))
    expect(yamlLineParts(lines[4]!, 4, index)).toContainEqual({ text: 'name', className: 'yaml-key' })
  })
  it('keeps a search match intact across multiple syntax spans', () => {
    const parts = yamlSearchParts(lines[0]!, 0, index, 'port: 7890')
    expect(parts.filter(part => part.match)).toHaveLength(1)
    expect(parts[0]!.tokens.map(token => token.text).join('')).toBe('port: 7890')
    expect(yamlSearchParts(lines[4]!, 4, index, 'port').filter(part => part.match)).toHaveLength(1)
  })
})
