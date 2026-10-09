<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { t } from '@/services/i18n'

const props = defineProps<{ modelValue: string[]; options: string[]; disabled: boolean; label: string; placeholder: string; searchPlaceholder: string; exclude?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()
const root = ref<HTMLElement | null>(null), trigger = ref<HTMLButtonElement | null>(null), panel = ref<HTMLElement | null>(null), searchInput = ref<HTMLInputElement | null>(null)
const open = ref(false), search = ref(''), panelStyle = ref<Record<string, string>>({})
const id = `group-member-select-${useId()}`
const selected = computed(() => new Set(props.modelValue))
// Retain references from copied groups even when they are absent from the candidate source.
const candidates = computed(() => [...new Set([...props.options, ...props.modelValue])].filter(value => value !== props.exclude || selected.value.has(value)))
const matches = computed(() => candidates.value.filter(value => value.toLocaleLowerCase().includes(search.value.trim().toLocaleLowerCase())))
const customOption = computed(() => {
  const value = search.value.trim()
  return value && value !== props.exclude && !candidates.value.includes(value) ? value : ''
})
// Search covers all candidates; opening a large subscription renders only the first page.
const visibleOptions = computed(() => customOption.value ? [customOption.value, ...matches.value.slice(0, 100)] : matches.value.slice(0, 100))
const preview = computed(() => props.modelValue.slice(0, 2).join('、') + (props.modelValue.length > 2 ? '…' : ''))
watch(() => props.disabled, disabled => { if (disabled) close() })
function toggle(value: string) {
  if (props.disabled) return
  if (selected.value.has(value)) emit('update:modelValue', props.modelValue.filter(item => item !== value))
  else if (value !== props.exclude) emit('update:modelValue', [...props.modelValue, value])
}
async function show() {
  if (props.disabled) return
  const rect = trigger.value?.getBoundingClientRect()
  if (!rect) return
  const width = Math.min(Math.max(rect.width, 240), window.innerWidth - 16)
  const below = window.innerHeight - rect.bottom - 14, above = rect.top - 14
  const useBelow = below >= 200 || below >= above
  panelStyle.value = { position: 'fixed', left: `${Math.max(8, Math.min(rect.left, window.innerWidth - width - 8))}px`, width: `${width}px`, maxHeight: `${Math.max(80, useBelow ? below : above)}px`, ...(useBelow ? { top: `${rect.bottom + 6}px` } : { bottom: `${window.innerHeight - rect.top + 6}px` }) }
  search.value = ''; open.value = true
  await nextTick()
  searchInput.value?.focus()
}
function close(restoreFocus = false) {
  open.value = false
  if (restoreFocus) trigger.value?.focus()
}
function keyboard(event: KeyboardEvent) {
  if (event.key === 'Escape' && open.value) { event.preventDefault(); event.stopPropagation(); close(true) }
  else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault(); event.stopPropagation()
    if (!open.value) { void show(); return }
    const inputs = [...(panel.value?.querySelectorAll<HTMLInputElement>('input[type="checkbox"]') || [])]
    if (!inputs.length) return
    const current = inputs.indexOf(document.activeElement as HTMLInputElement)
    const index = current < 0 ? (event.key === 'ArrowDown' ? 0 : inputs.length - 1) : (current + (event.key === 'ArrowDown' ? 1 : -1) + inputs.length) % inputs.length
    inputs[index]?.focus(); inputs[index]?.scrollIntoView({ block: 'nearest' })
  } else if (event.key === 'Enter' && open.value) {
    event.preventDefault(); event.stopPropagation()
    const current = document.activeElement as HTMLInputElement
    const value = current?.type === 'checkbox' ? current.value : visibleOptions.value[0]
    if (value) toggle(value)
  }
}
function contains(target: Node) { return root.value?.contains(target) || panel.value?.contains(target) }
function outside(event: PointerEvent) { if (event.target instanceof Node && !contains(event.target)) close() }
function scrolled(event: Event) { if (!(event.target instanceof Node) || !panel.value?.contains(event.target)) close() }
function focusOut(event: FocusEvent) { if (!(event.relatedTarget instanceof Node) || !contains(event.relatedTarget)) close() }
onMounted(() => { document.addEventListener('pointerdown', outside, true); document.addEventListener('scroll', scrolled, true); window.addEventListener('resize', scrolled) })
onBeforeUnmount(() => { document.removeEventListener('pointerdown', outside, true); document.removeEventListener('scroll', scrolled, true); window.removeEventListener('resize', scrolled) })
</script>

<template>
  <div ref="root" class="policy-select group-member-select" @keydown="keyboard" @focusout="focusOut">
    <button ref="trigger" type="button" class="policy-select-trigger" :disabled="disabled" :aria-label="label" aria-haspopup="dialog" :aria-expanded="open" :aria-controls="`${id}-panel`" @click="open ? close() : show()">
      <span>{{ preview || placeholder }}</span><span aria-hidden="true">⌄</span>
    </button>
    <Teleport to="body"><div v-if="open" :id="`${id}-panel`" ref="panel" class="policy-select-panel group-member-panel" :style="panelStyle" role="dialog" :aria-label="t('{arg0}候选', { arg0: label })" @keydown="keyboard" @focusout="focusOut">
      <input ref="searchInput" v-model="search" :aria-label="t('搜索{arg0}', { arg0: label })" :placeholder="searchPlaceholder" autocomplete="off" :aria-controls="`${id}-list`" />
      <div :id="`${id}-list`" class="group-member-options">
        <label v-for="option in visibleOptions" :key="option" class="group-member-option" :class="{ 'is-selected': selected.has(option) }">
          <input type="checkbox" :value="option" :checked="selected.has(option)" :disabled="disabled" :aria-label="option" @change="toggle(option)" />
          <span :title="option">{{ option === customOption ? t('使用“{arg0}”', { arg0: option }) : option }}</span>
        </label>
        <p v-if="!visibleOptions.length" class="policy-select-empty">{{ t('没有匹配的选项') }}</p>
      </div>
      <p v-if="matches.length > 100" class="policy-select-empty">{{ t('显示前 100 项，请搜索缩小范围') }}</p>
    </div></Teleport>
  </div>
</template>

<style scoped>
.group-member-select{min-width:0;width:100%}
.group-member-options{min-height:0;max-height:min(240px,30dvh);overflow:auto;scrollbar-width:thin;scrollbar-color:var(--muted) transparent}
.group-member-option{display:flex;align-items:center;gap:8px;padding:8px;border-radius:6px;font-size:12px;cursor:pointer;min-width:0}
.group-member-option:hover,.group-member-option:focus-within{background:color-mix(in srgb,var(--accent) 12%,var(--panel))}
.group-member-option.is-selected{color:var(--accent);background:color-mix(in srgb,var(--accent) 8%,var(--panel))}
.group-member-option input[type=checkbox]{flex:none;width:15px;height:15px;min-height:0;padding:0;margin:0;accent-color:var(--accent)}
.group-member-option span{min-width:0;overflow-wrap:anywhere}
</style>
