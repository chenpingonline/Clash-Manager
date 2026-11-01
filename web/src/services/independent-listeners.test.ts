import { describe, expect, it } from 'vitest'
import { availableListenerName, availableListenerPort, copyListener, listenerID } from './independent-listeners'
import type { IndependentListener } from '@/types/api'
const entry: IndependentListener = { id: 'a', name: 'US', type: 'mixed', listen: '127.0.0.1', port: 1080, proxy: 'US', enabled: true, users: [{ username: 'user', password: 'test' }] }
describe('independent listener drafts', () => {
  it('avoids managed, subscription and main ports, including disabled drafts', () => {
    expect(availableListenerPort([entry, { ...entry, port: 1082, enabled: false }], [1081, 7890])).toBe(1083)
    expect(availableListenerPort([], [65535], 65535)).toBeNull()
  })
  it('finds a unique name and does not alias authentication drafts', () => {
    expect(availableListenerName('US', [entry, { ...entry, name: 'US (2)' }])).toBe('US (3)')
    const draft = copyListener(entry)
    draft.users[0]!.password = 'changed'
    expect(entry.users[0]!.password).toBe('test')
    expect(draft.udp).toBe(true)
    expect(copyListener({ ...entry, udp: false }).udp).toBe(false)
    expect(copyListener({ ...entry, type: 'http', udp: true }).udp).toBe(false)
  })
  it('generates independent valid IDs', () => {
    const a = listenerID(), b = listenerID()
    expect(a).toMatch(/^[a-zA-Z0-9-]{1,64}$/)
    expect(a).not.toBe(b)
  })
})
