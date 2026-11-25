<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { t } from '@/services/i18n'

const props = defineProps<{ host: string; busy: boolean }>()
const emit = defineEmits<{ save: [host: string] }>()
const loopback = '127.0.0.1'
const custom = ref(false), draft = ref('')
function reset() {
  custom.value = props.host !== loopback
  draft.value = custom.value ? props.host : ''
}
watch(() => props.host, reset, { immediate: true })
const nextHost = computed(() => custom.value ? draft.value.trim() : loopback)
const changed = computed(() => nextHost.value !== props.host)
</script>

<template>
  <form class="controller-listen" @submit.prevent="changed && nextHost && !busy && emit('save', nextHost)">
    <div class="controller-custom">
      <input id="controller-custom" v-model="custom" type="checkbox" :disabled="busy" aria-controls="controller-host">
      <label for="controller-custom">{{ t('自定义监听地址') }}</label>
    </div>
    <div v-if="custom" class="controller-field">
      <label for="controller-host">{{ t('监听 IP') }}</label>
      <input id="controller-host" v-model="draft" class="mono" type="text" spellcheck="false" autocomplete="off" :disabled="busy" :placeholder="t('本机网卡 IP')" required>
    </div>
    <div v-if="changed" class="controller-actions">
      <span>{{ t('保存后将重启运行中的内核。') }}</span>
      <button class="ghost" type="button" :disabled="busy" @click="reset">{{ t('取消') }}</button>
      <button type="submit" :disabled="busy || !nextHost">{{ t(busy ? '保存中…' : '保存') }}</button>
    </div>
  </form>
</template>

<style scoped>
.controller-listen { grid-column: 1 / -1; display: grid; gap: 8px; min-width: 0; }
.controller-custom, .controller-field, .controller-actions { display: flex; align-items: center; gap: 8px; }
.controller-custom label, .controller-field label { font-weight: 600; font-size: 12px; color: var(--text); }
.controller-custom label, .controller-custom input { cursor: pointer; }
.controller-custom input[type="checkbox"] { width: 16px; min-width: 16px; height: 16px; min-height: 16px; margin: 0; padding: 0; box-shadow: none; accent-color: var(--accent); }
.controller-custom input:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.controller-field input { width: min(100%, 22em); min-width: 0; min-height: 32px; height: 32px; }
.controller-field label { flex: none; }
.controller-actions { flex-wrap: wrap; }
.controller-actions span { font-size: 12px; color: var(--muted); margin-inline-end: auto; }
.controller-actions button { min-height: 30px; padding: 4px 10px; }
</style>
