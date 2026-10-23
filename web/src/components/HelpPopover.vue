<script setup lang="ts">
import { t } from '@/services/i18n'

import { nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'

defineProps<{
  label: string
  triggerText?: string
}>()

const open = ref(false)
const panelId = `help-popover-${useId()}`
const trigger = ref<HTMLButtonElement | null>(null)
const panel = ref<HTMLElement | null>(null)
const position = ref<Record<string, string>>({})
let closeTimer: ReturnType<typeof setTimeout> | null = null

function cancelClose() {
  if (closeTimer) clearTimeout(closeTimer)
  closeTimer = null
}

function scheduleClose() {
  cancelClose()
  closeTimer = setTimeout(() => {
    open.value = false
    closeTimer = null
  }, 120)
}

function place() {
  const rect = trigger.value?.getBoundingClientRect()
  if (!rect) return
  const width = Math.min(360, window.innerWidth - 24)
  const left = Math.max(12, Math.min(rect.left - 7, window.innerWidth - width - 12))
  const top = rect.bottom + 8
  position.value = {
    left: `${left}px`,
    top: `${top}px`,
    width: `${width}px`,
    maxHeight: `${Math.max(120, window.innerHeight - top - 16)}px`,
  }
}

async function show() {
  await nextTick()
  place()
  document.addEventListener('pointerdown', outside, true)
  window.addEventListener('resize', place)
  window.addEventListener('scroll', place, true)
}

function hide() {
  document.removeEventListener('pointerdown', outside, true)
  window.removeEventListener('resize', place)
  window.removeEventListener('scroll', place, true)
}

function outside(event: PointerEvent) {
  const target = event.target
  if (!(target instanceof Node)) return
  if (trigger.value?.contains(target) || panel.value?.contains(target)) return
  close()
}

function toggle() {
  cancelClose()
  open.value = !open.value
}

function close() {
  cancelClose()
  open.value = false
}

watch(open, value => { value ? void show() : hide() }, { flush: 'post' })

onBeforeUnmount(() => {
  cancelClose()
  hide()
})
</script>

<template>
  <span class="help-popover" @mouseenter="cancelClose" @mouseleave="scheduleClose" @focusout="scheduleClose" @keydown.esc="close">
    <button
      ref="trigger"
      class="help-popover-trigger"
      :class="{ 'help-popover-text-trigger': triggerText }"
      type="button"
      :aria-label="t('{arg0}说明', { arg0: t(label) })"
      aria-haspopup="dialog"
      :aria-expanded="open"
      :aria-controls="panelId"
      @click="toggle"
    >{{ triggerText || '?' }}</button>
    <Teleport to="body">
      <span
        v-if="open"
        :id="panelId"
        ref="panel"
        class="help-popover-panel"
        role="dialog"
        :aria-label="t('{arg0}说明', { arg0: t(label) })"
        :style="position"
        @mouseenter="cancelClose"
        @mouseleave="scheduleClose"
      >
        <slot />
      </span>
    </Teleport>
  </span>
</template>

<style scoped>
.help-popover-text-trigger { display:inline-flex;width:auto;height:auto;padding:2px 0;border:0;border-radius:4px;font-size:12px;font-weight:400;line-height:1.5; }
</style>
