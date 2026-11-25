<script setup lang="ts">
import { computed, ref } from 'vue'
import AppSelect from '@/components/AppSelect.vue'
import BaseModal from '@/components/BaseModal.vue'
import { errorMessage } from '@/services/api'
import { t } from '@/services/i18n'

const props = defineProps<{ host: string; busy: boolean; save: (host: string) => Promise<void> }>()
const loopback = '127.0.0.1', allIPv4 = '0.0.0.0'
const mode = ref<'local' | 'lan' | 'custom'>('local')
const open = ref(false), saving = ref(false), draft = ref(''), error = ref('')
const address = computed(() => mode.value === 'local' ? loopback : mode.value === 'lan' ? allIPv4 : draft.value.trim())

function show() {
  mode.value = props.host === loopback ? 'local' : props.host === allIPv4 ? 'lan' : 'custom'
  draft.value = mode.value === 'custom' ? props.host : ''
  error.value = ''
  open.value = true
}
async function submit() {
  if (saving.value || props.busy || !address.value || address.value === props.host) return
  error.value = ''
  saving.value = true
  try {
    await props.save(address.value)
    open.value = false
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <button type="button" class="ghost controller-listen-trigger" :aria-label="t('Controller 监听设置')"
    aria-haspopup="dialog" :aria-expanded="open" :disabled="busy" @click="show">{{ t('监听设置') }}</button>
  <BaseModal :open="open" :title="t('Controller 监听设置')" :closable="!saving" :show-header="false"
    card-class="controller-listen-modal" @close="open = false">
    <form @submit.prevent="submit">
      <div class="settings-modal-header">
        <h3>{{ t('Controller 监听设置') }}</h3>
        <div class="settings-modal-header-actions">
          <button type="button" class="ghost" :disabled="saving" @click="open = false">{{ t('取消') }}</button>
          <button type="submit" :disabled="busy || saving || !address || address === host">{{ t(saving ? '保存中…' : '保存') }}</button>
        </div>
      </div>
      <div class="listen-fields">
        <label for="controller-listen-mode">{{ t('监听地址') }}</label>
        <AppSelect id="controller-listen-mode" autofocus v-model="mode" :label="t('监听地址')" :disabled="saving"
          :options="[{ value: 'local', label: t('仅本机') }, { value: 'lan', label: t('所有 IPv4 网卡') }, { value: 'custom', label: t('指定地址') }]" @change="error = ''" />
        <template v-if="mode === 'custom'">
          <label for="controller-host">{{ t('监听 IP') }}</label>
          <input id="controller-host" v-model="draft" class="mono" type="text" spellcheck="false" autocomplete="off"
            :disabled="saving" :placeholder="t('IPv4 或 IPv6 地址')" required @input="error = ''">
        </template>
      </div>
      <p v-if="error" class="error-text" role="alert">{{ t(error) }}</p>
      <p v-else-if="address && address !== host" class="restart-note">{{ t('保存后将重启运行中的内核。') }}</p>
    </form>
  </BaseModal>
</template>

<style scoped>
.controller-listen-trigger.ghost { min-height: 24px; padding: 3px 6px; font-size: 11px; color: var(--accent); }
:global(.modal-card.controller-listen-modal) { width: min(420px, 100%); padding: 18px; }
/* Keep the fixed-position option panel anchored during the modal fade-in. */
:global(.modal-motion-enter-active .modal-card.controller-listen-modal) { transform: none; }
.settings-modal-header { flex-wrap: wrap; }
.settings-modal-header h3 { margin: 0; }
.listen-fields { display: grid; grid-template-columns: max-content minmax(0, 1fr); align-items: center; gap: 10px 8px; margin-top: 14px; }
.listen-fields label { text-align: right; font-weight: 600; color: var(--text); }
.listen-fields input { min-width: 0; width: 100%; }
.restart-note, .error-text { margin: 12px 0 0; }
.restart-note { color: var(--muted); }
</style>
