<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'

const props = withDefaults(defineProps<{ open: boolean; title: string; closable?: boolean; cardClass?: string; showHeader?: boolean; inert?: boolean }>(), { closable: true, cardClass: '', showHeader: true, inert: false })
const emit = defineEmits<{ close: [] }>()

const card = ref<HTMLElement | null>(null)
let restoreFocus: HTMLElement | null = null

// The app shell is deliberately not made inert: toasts live inside it and must
// stay operable. aria-modal already hides the background from assistive tech.
function focusable(): HTMLElement[] {
  const selector = 'a[href],button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled),summary,[tabindex]:not([tabindex="-1"])'
  return Array.from(card.value?.querySelectorAll<HTMLElement>(selector) || [])
    .filter(element => element.tabIndex >= 0 && element.getClientRects().length > 0)
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    if (!props.closable) return
    event.preventDefault()
    event.stopPropagation()
    emit('close')
    return
  }
  if (event.key !== 'Tab') return
  const items = focusable()
  if (!items.length) {
    event.preventDefault()
    card.value?.focus()
    return
  }
  const first = items[0]!
  const last = items[items.length - 1]!
  const active = document.activeElement
  if (event.shiftKey && (active === first || active === card.value)) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && (active === last || active === card.value)) {
    event.preventDefault()
    first.focus()
  }
}

watch(() => props.open, async (open, wasOpen) => {
  if (open) {
    if (!wasOpen) restoreFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
    await nextTick()
    const preferred = card.value?.querySelector<HTMLElement>('[autofocus]')
    ;(preferred || focusable()[0] || card.value)?.focus()
    return
  }
  if (wasOpen) {
    restoreFocus?.focus?.()
    restoreFocus = null
  }
})

onBeforeUnmount(() => {
  restoreFocus?.focus?.()
  restoreFocus = null
})
</script>

<template>
  <Teleport to="body">
    <Transition name="modal-motion">
      <div v-if="open" class="modal" role="presentation" @click.self="props.closable && emit('close')" @keydown="onKeydown">
        <section ref="card" class="modal-card" :class="props.cardClass" role="dialog" aria-modal="true" :aria-label="title" tabindex="-1" :inert="inert || undefined" :aria-hidden="inert || undefined">
          <template v-if="showHeader"><slot name="header"><h3>{{ title }}</h3></slot></template>
          <slot />
        </section>
      </div>
    </Transition>
  </Teleport>
</template>
