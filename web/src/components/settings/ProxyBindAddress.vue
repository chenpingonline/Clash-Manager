<script setup lang="ts">
import { nextTick, ref, useId } from 'vue'
import BaseModal from '@/components/BaseModal.vue'
import { errorMessage } from '@/services/api'
import { t } from '@/services/i18n'

const props = defineProps<{ modelValue: string; allowLan: boolean; busy: boolean; save: (address: string) => Promise<void> }>()
const emit = defineEmits<{ 'update:allowLan': [value: boolean]; lanChange: [] }>()
const inputId = `proxy-lan-${useId()}`
const open = ref(false), saving = ref(false), error = ref('')
const mode = ref('all'), customAddress = ref('')
const trigger = ref<HTMLButtonElement>(), form = ref<HTMLFormElement>(), modeInput = ref<HTMLSelectElement>()
async function show() {
  mode.value = props.modelValue === '*' ? 'all' : 'ip'
  customAddress.value = props.modelValue === '*' ? '' : props.modelValue
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
  const controls = Array.from(form.value?.querySelectorAll<HTMLElement>('button:not(:disabled), select:not(:disabled), input:not(:disabled)') || [])
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
  <BaseModal :open="open" :title="t('代理监听地址')" :show-header="false" :closable="!saving" card-class="proxy-bind-modal" @close="close">
    <form ref="form" tabindex="-1" @submit.prevent="saveAddress" @keydown="modalKeydown">
      <div class="settings-modal-header">
        <h3>{{ t('代理监听地址') }}</h3>
        <div class="settings-modal-header-actions"><button class="ghost" type="button" :disabled="saving" @click="close">{{ t('取消') }}</button><button type="submit" :disabled="saving || !allowLan">{{ t(saving ? '保存中…' : '保存') }}</button></div>
      </div>
      <p class="field-note">{{ t('控制代理端口监听的地址，不影响 Controller API。') }}</p>
      <label class="bind-field"><span>{{ t('监听地址') }}</span><select ref="modeInput" v-model="mode" :disabled="saving || !allowLan" @change="error = ''"><option value="all">{{ t('所有网卡') }}</option><option value="ip">{{ t('指定 IP') }}</option></select></label>
      <label v-if="mode === 'ip'" class="bind-field"><span>{{ t('指定监听 IP') }}</span><input v-model="customAddress" :disabled="saving || !allowLan" :placeholder="t('IPv4 或 IPv6 地址')" spellcheck="false" @input="error = ''"></label>
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
.bind-field { display: grid; grid-template-columns: 100px minmax(0, 1fr); align-items: center; gap: 10px; margin-top: 14px; }
.bind-field span { font-size: 12px; white-space: nowrap; }
.bind-field select, .bind-field input { width: 100%; min-width: 0; }
.field-note { margin: 14px 0; font-size: 11px; line-height: 1.6; color: var(--muted); }
.error-text { margin-bottom: 0; }
:global(.proxy-bind-modal) { width: min(460px, 100%); }
</style>
