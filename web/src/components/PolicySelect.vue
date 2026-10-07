<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'

const props = defineProps<{ modelValue: string; options: string[]; disabled: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const root = ref<HTMLElement | null>(null), trigger = ref<HTMLButtonElement | null>(null), searchInput = ref<HTMLInputElement | null>(null)
const open = ref(false), search = ref(''), activeIndex = ref(0)
const id = `policy-select-${useId()}`
const matches = computed(() => {
  const term = search.value.trim().toLocaleLowerCase()
  return props.options.filter(option => !term || option.toLocaleLowerCase().includes(term))
})
// Keep large subscriptions cheap to open; search still covers every candidate.
const visibleOptions = computed(() => matches.value.slice(0, 100))
watch(search, () => { activeIndex.value = 0 })
watch(() => props.options, () => { activeIndex.value = Math.min(activeIndex.value, Math.max(0, visibleOptions.value.length - 1)) })
watch(() => props.disabled, disabled => { if (disabled) open.value = false })
async function show() {
  if (props.disabled) return
  search.value = ''; open.value = true
  await nextTick()
  activeIndex.value = Math.max(0, visibleOptions.value.indexOf(props.modelValue))
  searchInput.value?.focus()
  await nextTick()
  root.value?.querySelector(`#${id}-option-${activeIndex.value}`)?.scrollIntoView({ block: 'nearest' })
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
  root.value?.querySelector(`#${id}-option-${activeIndex.value}`)?.scrollIntoView({ block: 'nearest' })
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
function outside(event: PointerEvent) {
  if (event.target instanceof Node && !root.value?.contains(event.target)) close()
}
function focusOut(event: FocusEvent) {
  if (!(event.relatedTarget instanceof Node) || !root.value?.contains(event.relatedTarget)) close()
}
onMounted(() => document.addEventListener('pointerdown', outside, true))
onBeforeUnmount(() => document.removeEventListener('pointerdown', outside, true))
</script>

<template>
  <div ref="root" class="rule-policy-field policy-select" @keydown="keyboard" @focusout="focusOut">
    <span :id="`${id}-label`">代理策略</span>
    <button ref="trigger" class="policy-select-trigger" type="button" :disabled="disabled" :aria-labelledby="`${id}-label`" aria-haspopup="listbox" :aria-expanded="open" :aria-controls="`${id}-list`" @click="open ? close() : show()">
      <span :title="modelValue">{{ modelValue || '选择代理策略' }}</span><span aria-hidden="true">⌄</span>
    </button>
    <div v-if="open" class="policy-select-panel">
      <input ref="searchInput" v-model="search" role="combobox" aria-label="搜索代理策略" placeholder="搜索代理组或节点" autocomplete="off" :aria-expanded="open" :aria-controls="`${id}-list`" :aria-activedescendant="visibleOptions.length ? `${id}-option-${activeIndex}` : undefined" />
      <div :id="`${id}-list`" class="policy-select-options" role="listbox" aria-label="代理策略候选">
        <button v-for="(option, index) in visibleOptions" :id="`${id}-option-${index}`" :key="option" type="button" role="option" :aria-selected="option === modelValue" :class="{ 'is-active': index === activeIndex, 'is-selected': option === modelValue }" tabindex="-1" @pointerdown.prevent @click="choose(option)">
          <span :title="option">{{ option }}</span><span v-if="option === modelValue" aria-hidden="true">✓</span>
        </button>
        <p v-if="!matches.length" class="policy-select-empty">没有匹配的策略</p>
      </div>
      <p v-if="matches.length > visibleOptions.length" class="policy-select-empty">显示前 100 项，请搜索缩小范围</p>
    </div>
  </div>
</template>
