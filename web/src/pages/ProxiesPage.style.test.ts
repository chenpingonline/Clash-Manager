import { readFileSync } from 'node:fs'
import { compileStyle, parse } from '@vue/compiler-sfc'
import { describe, expect, it } from 'vitest'

// Regression for #17: :global(html[lang^="en"]) followed by a descendant
// selector compiled into html[lang^="en"] { width: 88px }, shrinking
// the entire fnOS app when switching to English.
describe('ProxiesPage language-specific styles', () => {
  it('does not shrink the document root with language-specific styles', () => {
    const source = readFileSync(new URL('./ProxiesPage.vue', import.meta.url), 'utf8')
    const { descriptor, errors } = parse(source, { filename: 'ProxiesPage.vue' })
    expect(errors).toEqual([])

    const css = descriptor.styles.map((style, index) => {
      const result = compileStyle({
        filename: 'ProxiesPage.vue',
        id: `data-v-proxy-style-${index}`,
        source: style.content,
        scoped: style.scoped,
      })
      expect(result.errors).toEqual([])
      return result.code
    }).join('\n')

    expect(css).not.toMatch(/html\[lang[^\]]+\]\s*\{[^}]*\bwidth\s*:/)
  })
})
