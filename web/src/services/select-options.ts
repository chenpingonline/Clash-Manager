export type SelectValue = string | number
export type SelectOption = { value: SelectValue; label: string | number; disabled?: boolean }

export function nextSelectIndex(options: readonly SelectOption[], current: number, direction: 1 | -1): number {
  if (!options.some(option => !option.disabled)) return -1
  for (let step = 1; step <= options.length; step++) {
    const index = ((current + direction * step) % options.length + options.length) % options.length
    if (!options[index]?.disabled) return index
  }
  return -1
}
export function searchSelectIndex(options: readonly SelectOption[], text: string, current: number): number {
  const query = text.toLocaleLowerCase()
  for (let step = 1; step <= options.length; step++) {
    const index = (current + step + options.length) % options.length
    const option = options[index]
    if (option && !option.disabled && String(option.label).toLocaleLowerCase().startsWith(query)) return index
  }
  return -1
}
