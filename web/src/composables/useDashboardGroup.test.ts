import { describe, expect, it } from 'vitest'
import { useDashboardGroup } from './useDashboardGroup'

function memoryStorage() {
  const items = new Map<string, string>()
  return { getItem: (key: string) => items.get(key) ?? null, setItem: (key: string, value: string) => { items.set(key, value) } }
}

describe('dashboard strategy group choice', () => {
  it('restores automatic selection across page remounts and new sessions', () => {
    const storage = memoryStorage()
    const first = useDashboardGroup(storage)
    first.reconcileGroups(['Manual', 'Automatic'])
    first.chooseGroup('Automatic')
    const reopened = useDashboardGroup(storage)
    reopened.reconcileGroups(['Manual', 'Automatic'])
    expect(reopened.selectedGroupName.value).toBe('Automatic')
  })

  it('keeps the saved choice during an empty/offline response and falls back when a group is removed', () => {
    const storage = memoryStorage()
    const selection = useDashboardGroup(storage)
    selection.chooseGroup('Automatic')
    selection.reconcileGroups([])
    expect(useDashboardGroup(storage).selectedGroupName.value).toBe('Automatic')
    selection.reconcileGroups(['New group'])
    expect(useDashboardGroup(storage).selectedGroupName.value).toBe('New group')
  })

  it('continues to work when embedded browser storage is denied', () => {
    const selection = useDashboardGroup({ getItem: () => { throw new Error('denied') }, setItem: () => { throw new Error('denied') } })
    selection.reconcileGroups(['Manual', 'Automatic'])
    selection.chooseGroup('Automatic')
    selection.reconcileGroups(['Manual', 'Automatic'])
    expect(selection.selectedGroupName.value).toBe('Automatic')
  })
})
