<script setup lang="ts">
import { t } from '@/services/i18n'

import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'

const props = withDefaults(defineProps<{ modelValue: string; options: string[]; disabled: boolean; label?: string; placeholder?: string; searchPlaceholder?: string; hideLabel?: boolean; allowCustom?: boolean; panelClass?: string }>(), { label: '代理策略', placeholder: '选择代理策略', searchPlaceholder: '搜索代理组或节点', hideLabel: false, allowCustom: false, panelClass: '' })
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const root = ref<HTMLElement | null>(null), trigger = ref<HTMLButtonElement | null>(null), searchInput = ref<HTMLInputElement | null>(null)
const panel = ref<HTMLElement | null>(null)
const panelStyle = ref<Record<string, string>>({})
const open = ref(false), search = ref(''), activeIndex = ref(0)
const id = `policy-select-${useId()}`
const matches = computed(() => {
  const term = search.value.trim().toLocaleLowerCase()
  return props.options.filter(option => !term || option.toLocaleLowerCase().includes(term))
})
// Keep large subscriptions cheap to open; search still covers every candidate.
const customOption = computed(() => props.allowCustom && search.value.trim() && !props.options.includes(search.value.trim()) ? search.value.trim() : '')
const visibleOptions = computed(() => customOption.value ? [customOption.value, ...matches.value.slice(0, 100)] : matches.value.slice(0, 100))
watch(search, () => { activeIndex.value = 0 })
watch(() => props.options, () => { activeIndex.value = Math.min(activeIndex.value, Math.max(0, visibleOptions.value.length - 1)) })
watch(() => props.disabled, disabled => { if (disabled) open.value = false })
async function show() {
  if (props.disabled) return
  const rect = trigger.value?.getBoundingClientRect()
  if (!rect) return
  const width = Math.min(Math.max(rect.width, 240), window.innerWidth - 16)
  const below = window.innerHeight - rect.bottom - 14
  const above = rect.top - 14
  const useBelow = below >= 200 || below >= above
  panelStyle.value = { position: 'fixed', left: `${Math.max(8, Math.min(rect.left, window.innerWidth - width - 8))}px`, width: `${width}px`, right: 'auto', maxHeight: `${Math.max(80, useBelow ? below : above)}px`, ...(useBelow ? { top: `${rect.bottom + 6}px` } : { top: 'auto', bottom: `${window.innerHeight - rect.top + 6}px` }) }
  search.value = ''; open.value = true
  await nextTick()
  activeIndex.value = Math.max(0, visibleOptions.value.indexOf(props.modelValue))
  searchInput.value?.focus()
  await nextTick()
  panel.value?.querySelector(`#${id}-option-${activeIndex.value}`)?.scrollIntoView({ block: 'nearest' })
}
function close(restoreFocus = false) {
  open.value = false
  if (restoreFocus) trigger.value?.focus()
}
function choose(value: string) { emit('update:modelValue', value); close(true) }
async function move(delta: number) {
  const count = visibleOptions.value.length
  if (!count) return
  activeIndex.value = (activeIndex.value + delta + count) % count
  await nextTick()
  panel.value?.querySelector(`#${id}-option-${activeIndex.value}`)?.scrollIntoView({ block: 'nearest' })
}
function keyboard(event: KeyboardEvent) {
  if (event.key === 'Escape' && open.value) { event.preventDefault(); event.stopPropagation(); close(true) }
  else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault(); event.stopPropagation()
    if (!open.value) void show(); else void move(event.key === 'ArrowDown' ? 1 : -1)
  } else if (event.key === 'Enter' && open.value) {
    event.preventDefault(); event.stopPropagation()
    const value = visibleOptions.value[activeIndex.value]
    if (value) choose(value)
  }
}
function contains(target: Node) { return root.value?.contains(target) || panel.value?.contains(target) }
function outside(event: PointerEvent) {
  if (event.target instanceof Node && !contains(event.target)) close()
}
function scrolled(event: Event) { if (!(event.target instanceof Node) || !panel.value?.contains(event.target)) close() }
function focusOut(event: FocusEvent) {
  if (!(event.relatedTarget instanceof Node) || !contains(event.relatedTarget)) close()
}
onMounted(() => { document.addEventListener('pointerdown', outside, true); document.addEventListener('scroll', scrolled, true); window.addEventListener('resize', scrolled) })
onBeforeUnmount(() => { document.removeEventListener('pointerdown', outside, true); document.removeEventListener('scroll', scrolled, true); window.removeEventListener('resize', scrolled) })
</script>

<template>
  <div ref="root" class="rule-policy-field policy-select" @keydown="keyboard" @focusout="focusOut">
    <span v-if="!hideLabel" :id="`${id}-label`">{{ t(label) }}</span>
    <button ref="trigger" class="policy-select-trigger" type="button" :disabled="disabled" :aria-labelledby="hideLabel ? undefined : `${id}-label`" :aria-label="t(hideLabel ? label : undefined)" aria-haspopup="listbox" :aria-expanded="open" :aria-controls="`${id}-list`" @click="open ? close() : show()">
      <span :title="modelValue">{{ modelValue || t(placeholder) }}</span><span aria-hidden="true">⌄</span>
    </button>
    <Teleport to="body"><div v-if="open" ref="panel" class="policy-select-panel" :class="panelClass" :style="panelStyle" @keydown="keyboard" @focusout="focusOut">
      <input ref="searchInput" v-model="search" role="combobox" :aria-label="t('搜索{arg0}', { arg0: t(label) })" :placeholder="t(searchPlaceholder)" autocomplete="off" :aria-expanded="open" :aria-controls="`${id}-list`" :aria-activedescendant="visibleOptions.length ? `${id}-option-${activeIndex}` : undefined" />
      <div :id="`${id}-list`" class="policy-select-options" role="listbox" :aria-label="t('{arg0}候选', { arg0: t(label) })">
        <button v-for="(option, index) in visibleOptions" :id="`${id}-option-${index}`" :key="option" type="button" role="option" :aria-selected="option === modelValue" :class="{ 'is-active': index === activeIndex, 'is-selected': option === modelValue }" tabindex="-1" @pointerdown.prevent @click="choose(option)">
          <span :title="option">{{ option === customOption ? t('使用“{arg0}”', { arg0: option }) : option }}</span><span v-if="option === modelValue" aria-hidden="true">✓</span>
        </button>
        <p v-if="!visibleOptions.length" class="policy-select-empty">{{ t("没有匹配的选项") }}</p>
      </div>
      <p v-if="matches.length > 100" class="policy-select-empty">{{ t("显示前 100 项，请搜索缩小范围") }}</p>
    </div></Teleport>
  </div>
</template>
