<script setup lang="ts" generic="T extends string | number">
import { computed, nextTick, onBeforeUnmount, onUnmounted, ref, shallowRef, useId, watch } from 'vue'
import { Check, ChevronDown } from 'lucide-vue-next'
import { nextSelectIndex, searchSelectIndex, type SelectOption } from '@/services/select-options'

const props = withDefaults(defineProps<{ modelValue: T; options: SelectOption[]; disabled?: boolean; placeholder?: string; modelModifiers?: { number?: boolean } }>(), { disabled: false, placeholder: '' })
const emit = defineEmits<{ 'update:modelValue': [value: T]; change: [value: T] }>()
const trigger = ref<HTMLButtonElement | null>(null), panel = ref<HTMLElement | null>(null)
const open = ref(false), active = ref(-1), position = ref<Record<string, string>>({})
const portalTarget = shallowRef<HTMLElement | null>(null)
const id = `app-select-${useId()}`
const selected = computed(() => props.options.find(option => option.value === props.modelValue))
let search = '', lastKey = 0
function cleanup() {
  document.removeEventListener('pointerdown', outside, true)
  document.removeEventListener('scroll', scrolled, true)
  window.removeEventListener('resize', close)
}
function close() { open.value = false; search = ''; cleanup() }
function outside(event: PointerEvent) {
  if (event.target instanceof Node && !trigger.value?.contains(event.target) && !panel.value?.contains(event.target)) close()
}
function scrolled(event: Event) { if (!(event.target instanceof Node) || !panel.value?.contains(event.target)) close() }
async function reveal() {
  await nextTick()
  panel.value?.querySelector(`#${id}-${active.value}`)?.scrollIntoView({ block: 'nearest' })
}
async function show(direction: 1 | -1 = 1) {
  if (props.disabled) return
  const rect = trigger.value?.getBoundingClientRect()
  if (!rect) return
  const parent = trigger.value?.closest<HTMLElement>('dialog, [role="dialog"]') || document.body
  if (portalTarget.value?.parentElement !== parent) {
    portalTarget.value?.remove()
    const host = document.createElement('div')
    host.className = 'app-select-portal'
    parent.appendChild(host)
    portalTarget.value = host
  }
  const width = Math.min(Math.max(rect.width, 150), window.innerWidth - 16)
  const below = window.innerHeight - rect.bottom - 14, above = rect.top - 14
  const useBelow = below >= Math.min(280, props.options.length * 32 + 10) || below >= above
  position.value = { left: `${Math.max(8, Math.min(rect.right - width, window.innerWidth - width - 8))}px`, width: `${width}px`, maxHeight: `${Math.max(40, Math.min(320, useBelow ? below : above))}px`, ...(useBelow ? { top: `${rect.bottom + 6}px` } : { bottom: `${window.innerHeight - rect.top + 6}px` }) }
  const selectedIndex = props.options.findIndex(option => option.value === props.modelValue && !option.disabled)
  active.value = selectedIndex >= 0 ? selectedIndex : nextSelectIndex(props.options, direction === 1 ? -1 : 0, direction)
  open.value = true
  document.addEventListener('pointerdown', outside, true)
  document.addEventListener('scroll', scrolled, true)
  window.addEventListener('resize', close)
  trigger.value?.focus()
  await reveal()
}
function choose(index: number) {
  const option = props.options[index]
  if (!option || option.disabled || props.disabled) return
  const value = (props.modelModifiers?.number ? Number(option.value) : option.value) as T
  const changed = value !== props.modelValue
  emit('update:modelValue', value)
  if (changed) emit('change', value)
  close()
  trigger.value?.focus()
}
async function keyboard(event: KeyboardEvent) {
  if (event.key === 'Tab') { close(); return }
  if (event.key === 'Escape') {
    if (open.value) { event.preventDefault(); event.stopPropagation(); close() }
    return
  }
  const action = ['ArrowDown', 'ArrowUp', 'Home', 'End', 'Enter', ' '].includes(event.key)
  const typing = event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey
  if (!action && !typing) return
  event.preventDefault(); event.stopPropagation()
  if (!open.value) {
    await show(event.key === 'ArrowUp' || event.key === 'End' ? -1 : 1)
    if (event.key === 'Enter' || event.key === ' ' || event.key.startsWith('Arrow')) return
  } else if (event.key === 'Enter' || (event.key === ' ' && !search)) { choose(active.value); return }
  if (event.key === 'Home') active.value = nextSelectIndex(props.options, -1, 1)
  else if (event.key === 'End') active.value = nextSelectIndex(props.options, 0, -1)
  else if (event.key.startsWith('Arrow')) active.value = nextSelectIndex(props.options, active.value, event.key === 'ArrowDown' ? 1 : -1)
  else if (typing) {
    const now = Date.now()
    search = now - lastKey > 700 ? event.key : search + event.key
    lastKey = now
    const index = searchSelectIndex(props.options, search, active.value)
    if (index >= 0) active.value = index
  }
  await reveal()
}
watch(() => props.disabled, disabled => { if (disabled) close() })
watch(() => props.options, options => {
  if (open.value && (!options[active.value] || options[active.value]?.disabled)) active.value = nextSelectIndex(options, -1, 1)
})
defineExpose({ focus: () => trigger.value?.focus() })
// Let Teleport remove its children before disposing of the target container.
onBeforeUnmount(cleanup)
onUnmounted(() => portalTarget.value?.remove())
</script>

<template>
  <button ref="trigger" type="button" class="app-select" role="combobox" :disabled="disabled" aria-haspopup="listbox" :aria-expanded="open" :aria-controls="open ? id : undefined" :aria-activedescendant="open && active >= 0 ? `${id}-${active}` : undefined" :title="String(selected?.label ?? placeholder)" @click.stop="open ? close() : show()" @keydown="keyboard" @blur="close">
    <span class="app-select-value">{{ selected?.label ?? placeholder }}</span><ChevronDown :size="14" :stroke-width="1.8" aria-hidden="true" />
    <Teleport v-if="open && portalTarget" :to="portalTarget">
      <div :id="id" ref="panel" class="app-select-menu" role="listbox" :aria-label="$attrs['aria-label'] as string | undefined" :style="position" @pointerdown.prevent @click.stop>
        <button v-for="(option, index) in options" :id="`${id}-${index}`" :key="`${typeof option.value}:${option.value}`" type="button" role="option" :aria-selected="modelValue === option.value" :aria-disabled="option.disabled || undefined" :disabled="option.disabled" :class="{ active: active === index }" tabindex="-1" @pointermove="!option.disabled && (active = index)" @click.stop="choose(index)">
          <span>{{ option.label }}</span><Check v-if="modelValue === option.value" :size="14" aria-hidden="true" />
        </button>
      </div>
    </Teleport>
  </button>
</template>

<style scoped>
.app-select { width: 100%; min-width: 0; min-height: 0; height: 34px; display: inline-flex; align-items: center; justify-content: space-between; gap: 8px; padding: 7px 10px; border: 1px solid var(--line); border-radius: 9px; background: var(--control-bg); color: var(--text); font-size: 12px; font-weight: 400; text-align: left; cursor: pointer; }
.app-select-value { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.app-select > svg { flex: none; }
.app-select:focus-visible, .app-select[aria-expanded="true"] { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb,var(--accent) 10%,transparent); }
.app-select:disabled { background: var(--surface-disabled); color: var(--muted); }
.app-select-menu { position: fixed; z-index: 1100; overflow-y: auto; overscroll-behavior: contain; padding: 4px; border: 1px solid var(--line); border-radius: 9px; background: var(--panel); color: var(--text); box-shadow: var(--shadow); }
.app-select-menu button { width: 100%; display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 7px 8px; border: 0; border-radius: 5px; background: transparent; color: var(--text); font-size: 12px; font-weight: 400; text-align: left; }
.app-select-menu button > span { min-width: 0; overflow-wrap: anywhere; }
.app-select-menu button > svg { flex: none; }
.app-select-menu button.active { background: var(--accent-soft); color: var(--accent-ink); }
</style>
