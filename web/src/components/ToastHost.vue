<script setup lang="ts">
import { toasts } from '@/services/toast'
</script>

<template>
  <div id="toast" aria-live="polite">
    <div v-for="item in toasts" :key="item.id" class="toast" :class="{ bad: item.bad }">
      <svg class="toast-status-icon" :class="item.bad ? 'toast-icon-error' : 'toast-icon-success'" viewBox="0 0 20 20" aria-hidden="true">
        <path v-if="item.bad" d="M5 5l10 10M15 5L5 15" />
        <path v-else d="M4 10l4 4 8-8" />
      </svg>
      <span class="toast-message">{{ item.text }}</span>
    </div>
  </div>
</template>

<style scoped>
.toast { display: flex; align-items: flex-start; gap: 9px; }
.toast-status-icon { width: 20px; height: 20px; margin-top: 1px; flex: none; fill: none; stroke: currentColor; stroke-width: 2.2; stroke-linecap: round; stroke-linejoin: round; }
.toast-status-icon.toast-icon-success { color: #209b57; }
.toast-status-icon.toast-icon-error { color: #e3475c; }
.toast-message { min-width: 0; line-height: 1.5; overflow-wrap: anywhere; }
</style>
