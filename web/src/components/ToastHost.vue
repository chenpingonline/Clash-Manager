<script setup lang="ts">
import { t } from '@/services/i18n'

import { dismissToast, toasts } from '@/services/toast'
import ConfigErrorNotice from './ConfigErrorNotice.vue'
</script>

<template>
  <div id="toast" aria-live="polite">
    <div v-for="item in toasts" :key="item.id" class="toast" :class="{ bad: item.bad, 'config-error-toast': item.configError }">
      <svg class="toast-status-icon" :class="item.bad ? 'toast-icon-error' : 'toast-icon-success'" viewBox="0 0 20 20" aria-hidden="true">
        <path v-if="item.bad" d="M5 5l10 10M15 5L5 15" />
        <path v-else d="M4 10l4 4 8-8" />
      </svg>
      <ConfigErrorNotice v-if="item.configError" class="toast-message" :message="item.text" />
      <span v-else class="toast-message">{{ t(item.text) }}</span>
      <button v-if="item.configError" type="button" class="toast-close" :aria-label="t('关闭')" @click="dismissToast(item.id)"><svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 4 8 8m0-8-8 8" /></svg></button>
    </div>
  </div>
</template>

<style scoped>
.toast { display: flex; align-items: flex-start; gap: 9px; }
.toast-status-icon { width: 20px; height: 20px; margin-top: 1px; flex: none; fill: none; stroke: currentColor; stroke-width: 2.2; stroke-linecap: round; stroke-linejoin: round; }
.toast-status-icon.toast-icon-success { color: #209b57; }
.toast-status-icon.toast-icon-error { color: #e3475c; }
.toast.config-error-toast{width:min(480px,calc(100vw - 44px));max-width:none;max-height:calc(100dvh - 44px);overflow:auto}
.toast-close{display:grid;place-items:center;flex:none;width:22px;height:22px;min-height:0;padding:3px;border:0;background:transparent;color:var(--muted)}
.toast-close svg{width:16px;height:16px;fill:none;stroke:currentColor;stroke-width:1.8;stroke-linecap:round}
.toast-close:focus-visible{outline:2px solid var(--accent)}
.toast-message { min-width: 0; line-height: 1.5; overflow-wrap: anywhere; }
</style>
