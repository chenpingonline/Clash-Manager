import { describe, expect, it } from 'vitest'
import { addAppDirectRules } from './installed-apps'
import { parseSequence, serializeSequence } from './profile-sequences'

const app = { id: 'music', name: 'Music', rule: 'PROCESS-PATH-REGEX,^/var/apps/music/target/,DIRECT' }
describe('installed application direct rules', () => {
  it('moves an app to the front, removes duplicates and retains unrelated edits', () => {
    const model = { prepend: ['DOMAIN,test,DIRECT'], append: [app.rule], delete: [app.rule, 'MATCH,DIRECT'], custom: true }
    const result = addAppDirectRules(model, [app, app])
    expect(result.prepend).toEqual([app.rule, 'DOMAIN,test,DIRECT'])
    expect(result.append).toEqual([])
    expect(result.delete).toEqual(['MATCH,DIRECT'])
    expect(result['find-process-mode']).toBe('always')
    expect(result.custom).toBe(true)
    expect(model.append).toEqual([app.rule])
    expect(addAppDirectRules(result, [app])).toEqual(result)
    expect(parseSequence(serializeSequence(result), 'rules')).toEqual(result)
  })
  it('does not enable process lookup when no usable app was selected', () => {
    const model = { prepend: [], append: [], delete: [] }
    expect(addAppDirectRules(model, [{ ...app, rule: '' }])).toBe(model)
  })
})
