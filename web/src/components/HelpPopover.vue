<script setup lang="ts">
import { t } from '@/services/i18n'

import { onBeforeUnmount, ref, useId } from 'vue'

defineProps<{
  label: string
}>()

const open = ref(false)
const panelId = `help-popover-${useId()}`
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

function toggle() {
  cancelClose()
  open.value = !open.value
}

function close() {
  cancelClose()
  open.value = false
}

onBeforeUnmount(cancelClose)
</script>

<template>
  <span class="help-popover" @mouseenter="cancelClose" @mouseleave="scheduleClose" @focusout="scheduleClose" @keydown.esc="close">
    <button
      class="help-popover-trigger"
      type="button"
      :aria-label="t('{arg0}说明', { arg0: t(label) })"
      aria-haspopup="dialog"
      :aria-expanded="open"
      :aria-controls="panelId"
      @click="toggle"
    >?</button>
    <span v-if="open" :id="panelId" class="help-popover-panel" role="dialog" :aria-label="t('{arg0}说明', { arg0: t(label) })">
      <slot />
    </span>
  </span>
</template>
