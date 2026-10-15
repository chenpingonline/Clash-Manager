<script setup lang="ts">
import AppSelect from '@/components/AppSelect.vue'
import { nextTick, ref, useId } from 'vue'
import BaseModal from '@/components/BaseModal.vue'
import { errorMessage } from '@/services/api'
import { t } from '@/services/i18n'

const props = defineProps<{ modelValue: string; allowLan: boolean; busy: boolean; save: (address: string) => Promise<void> }>()
const emit = defineEmits<{ 'update:allowLan': [value: boolean]; lanChange: [] }>()
const inputId = `proxy-lan-${useId()}`
const open = ref(false), saving = ref(false), error = ref('')
const mode = ref('all'), customAddress = ref('')
const trigger = ref<HTMLButtonElement>(), form = ref<HTMLFormElement>(), modeInput = ref<{ focus: () => void }>()
async function show() {
  const address = props.modelValue.trim() || '*'
  mode.value = address === '*' ? 'all' : 'ip'
  customAddress.value = address === '*' ? '' : address
  error.value = ''
  open.value = true
  await nextTick()
  modeInput.value?.focus()
}
async function close() {
  if (saving.value) return
  open.value = false
  await nextTick()
  trigger.value?.focus()
}
async function saveAddress() {
  if (saving.value || !props.allowLan) return
  const address = mode.value === 'all' ? '*' : customAddress.value.trim()
  error.value = address ? '' : '请输入监听 IP 地址'
  if (!address) return
  form.value?.focus()
  saving.value = true
  try {
    await props.save(address)
    saving.value = false
    await close()
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    saving.value = false
    if (open.value) { await nextTick(); modeInput.value?.focus() }
  }
}
function modalKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') { event.preventDefault(); void close() }
  if (event.key !== 'Tab') return
  const controls = Array.from(form.value?.querySelectorAll<HTMLElement>('button:not(:disabled), select:not(:disabled), input:not(:disabled)') || []).filter(element => element.tabIndex >= 0 && !element.matches(':disabled') && element.getClientRects().length)
  const first = controls[0], last = controls[controls.length - 1]
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
}
</script>

<template>
  <div class="setting-toggle lan-setting">
    <div class="lan-copy">
      <div class="lan-title">
        <label :for="inputId"><strong>{{ t('允许局域网连接') }}</strong></label>
        <button ref="trigger" class="ghost bind-trigger" type="button" :aria-label="t('监听设置')" :title="t('监听设置')" aria-haspopup="dialog" :aria-expanded="open" :disabled="!allowLan || busy" @click="show">
          {{ t('监听设置') }}
        </button>
      </div>
      <label class="lan-description" :for="inputId">{{ t('允许其他设备访问已启用的代理端口') }}</label>
    </div>
    <label class="switch"><input :id="inputId" type="checkbox" :checked="allowLan" :aria-label="t('允许局域网连接')" @change="emit('update:allowLan', ($event.target as HTMLInputElement).checked); emit('lanChange')"><span /></label>
  </div>
  <BaseModal :open="open" :title="t('代理监听网卡设置')" :show-header="false" :closable="!saving" card-class="proxy-bind-modal" @close="close">
    <form ref="form" tabindex="-1" @submit.prevent="saveAddress" @keydown="modalKeydown">
      <div class="settings-modal-header">
        <h3>{{ t('代理监听网卡设置') }}</h3>
        <div class="settings-modal-header-actions"><button class="ghost" type="button" :disabled="saving" @click="close">{{ t('取消') }}</button><button type="submit" :disabled="saving || !allowLan">{{ t(saving ? '保存中…' : '保存') }}</button></div>
      </div>
      <p class="field-note">{{ t('默认监听所有网卡，也可填写 NAS 本机的网卡 IP。') }}</p>
      <div class="bind-fields">
        <label class="bind-field"><span>{{ t('监听地址') }}</span><AppSelect :aria-label="t('监听地址')" ref="modeInput" v-model="mode" :disabled="saving || !allowLan" @change="error = ''" :options="[{ value: 'all', label: (t('所有网卡')) }, { value: 'ip', label: (t('本机网卡 IP')) }]" /></label>
        <label v-if="mode === 'ip'" class="bind-field"><span>{{ t('指定监听 IP') }}</span><input v-model="customAddress" :disabled="saving || !allowLan" :placeholder="t('IPv4 或 IPv6 地址')" spellcheck="false" @input="error = ''"></label>
      </div>
      <p v-if="error" class="error-text" role="alert">{{ t(error) }}</p>
    </form>
  </BaseModal>
</template>

<style scoped>
.lan-setting { cursor: default; }
.lan-copy { min-width: 0; }
.lan-title { display: flex; align-items: center; flex-wrap: wrap; gap: 5px; }
.lan-title label, .lan-description { cursor: pointer; }
.lan-description { font-size: 10px; line-height: 1.45; color: var(--muted); }
.bind-trigger.ghost { display: inline-flex; align-items: center; justify-content: center; padding: 3px 6px; min-height: 24px; font-size: 10px; white-space: nowrap; border-radius: 6px; color: var(--accent); }
.bind-trigger.ghost:disabled { color: var(--muted); }
.bind-fields { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 12px; margin-top: 10px; }
.bind-field { display: inline-flex; align-items: center; gap: 8px; min-width: 0; max-width: 100%; }
.bind-field span { font-size: 12px; white-space: nowrap; }
.bind-field .app-select, .bind-field input { height: 28px; min-width: 0; padding: 4px 8px; border-radius: 7px; font-size: 12px; }
.bind-field .app-select { width: 110px; flex: none; }
.bind-field input { width: 180px; }
.field-note { margin: 10px 0; font-size: 11px; line-height: 1.6; color: var(--muted); }
.error-text { margin-bottom: 0; }
:global(.modal-card.proxy-bind-modal) { width: min(480px, 100%); padding: 14px 16px; }
</style>
