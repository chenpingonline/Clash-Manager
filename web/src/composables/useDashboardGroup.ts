import { ref } from 'vue'

const STORAGE_KEY = 'clash-for-fnos.dashboard-group.v1'

export function useDashboardGroup(storage?: Pick<Storage, 'getItem' | 'setItem'>) {
  // Browser storage may be unavailable in an embedded/private browsing context.
  try { storage ??= window.localStorage } catch { /* Keep an in-memory choice. */ }
  let saved = ''
  try { saved = storage?.getItem(STORAGE_KEY) || '' } catch { /* Use the first available group. */ }
  const selectedGroupName = ref(saved)
  function chooseGroup(name: string) {
    selectedGroupName.value = name
    try { storage?.setItem(STORAGE_KEY, name) } catch { /* The current choice still works. */ }
  }
  function reconcileGroups(names: string[]) {
    // An empty/offline response must not erase the last successful choice.
    if (names.length && !names.includes(selectedGroupName.value)) chooseGroup(names[0]!)
  }
  return { selectedGroupName, chooseGroup, reconcileGroups }
}
