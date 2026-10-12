<script setup lang="ts">
import { computed, nextTick, ref, useId } from 'vue'
import BaseModal from '@/components/BaseModal.vue'
import { errorMessage } from '@/services/api'
import { t } from '@/services/i18n'
import type { ProxyAccount, ProxyAuthentication } from '@/types/api'

const props = defineProps<{
  modelValue: ProxyAuthentication
  busy: boolean
  load: () => Promise<ProxyAuthentication>
  save: (value: ProxyAuthentication) => Promise<void>
}>()
const id = `proxy-auth-${useId()}`
const open = ref(false), loading = ref(false), saving = ref(false), error = ref('')
const loaded = ref(false)
const enabled = ref(false), passwordsVisible = ref(false)
const accounts = ref<ProxyAccount[]>([]), prefixes = ref('')
const trigger = ref<HTMLButtonElement>(), form = ref<HTMLFormElement>(), toggle = ref<HTMLInputElement>()
const blocked = computed(() => loading.value || saving.value)
const hasAuth = computed(() => props.modelValue.accounts.length > 0)

async function show() {
  if (props.busy) return
  open.value = true
  loading.value = true
  loaded.value = false
  error.value = ''
  passwordsVisible.value = false
  accounts.value = []
  prefixes.value = ''
  try {
    // Reload so manual YAML edits/profile changes are reflected even if this
    // settings page was already open. Never replace them with stale form data.
    const current = await props.load()
    accounts.value = current.accounts.map(account => ({ ...account }))
    enabled.value = accounts.value.length > 0
    prefixes.value = current.skipAuthPrefixes.join('\n')
    loaded.value = true
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    loading.value = false
    await nextTick()
    toggle.value?.focus()
  }
}
async function close() {
  if (blocked.value) return
  open.value = false
  accounts.value = []
  passwordsVisible.value = false
  await nextTick()
  trigger.value?.focus()
}
function toggleEnabled() {
  if (enabled.value && !accounts.value.length) accounts.value.push({ username: '', password: '' })
  error.value = ''
}
function addAccount() { accounts.value.push({ username: '', password: '' }); error.value = '' }
async function save() {
  if (blocked.value || !loaded.value) return
  error.value = ''
  if (enabled.value && (!accounts.value.length || accounts.value.some(account => !account.username.trim() || !account.password))) {
    error.value = '代理认证的用户名和密码不能为空'
    return
  }
  const value: ProxyAuthentication = {
    accounts: enabled.value ? accounts.value.map(account => ({ ...account })) : [],
    skipAuthPrefixes: prefixes.value.split(/\r?\n/).map(line => line.trim()).filter(Boolean),
  }
  saving.value = true
  try {
    await props.save(value)
    saving.value = false
    await close()
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally { saving.value = false }
}
function modalKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') { event.preventDefault(); void close() }
  if (event.key !== 'Tab') return
  const controls = Array.from(form.value?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), textarea:not(:disabled)') || [])
  const first = controls[0], last = controls[controls.length - 1]
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
}
</script>

<template>
  <div class="setting-toggle auth-setting">
    <div><strong>{{ t('代理认证') }} <span class="auth-status">{{ t(hasAuth ? '已启用' : '未启用') }}</span></strong><small>{{ t('HTTP、SOCKS5 与混合代理端口的用户名和密码') }}</small></div>
    <button ref="trigger" class="ghost" type="button" :disabled="busy" aria-haspopup="dialog" :aria-expanded="open" @click="show">{{ t('认证设置') }}</button>
  </div>
  <BaseModal :open="open" :title="t('代理认证设置')" :show-header="false" :closable="!blocked" card-class="proxy-auth-modal" @close="close">
    <form ref="form" @submit.prevent="save" @keydown="modalKeydown">
      <div class="settings-modal-header">
        <h3>{{ t('代理认证设置') }}</h3>
        <div class="settings-modal-header-actions"><button class="ghost" type="button" :disabled="blocked" @click="close">{{ t('取消') }}</button><button type="submit" :disabled="blocked || !loaded">{{ t(saving ? '保存中…' : '保存') }}</button></div>
      </div>
      <p class="field-note">{{ t('读取当前配置中的认证；保存后应用到代理端口，订阅更新时继续保留。') }}</p>
      <p v-if="loading" role="status">{{ t('正在读取代理认证配置…') }}</p>
      <template v-else-if="loaded">
        <label class="auth-toggle" :for="`${id}-enabled`"><strong>{{ t('启用代理认证') }}</strong><span class="switch"><input :id="`${id}-enabled`" ref="toggle" v-model="enabled" type="checkbox" :disabled="blocked" @change="toggleEnabled"><span /></span></label>
        <template v-if="enabled">
          <div class="auth-account-list">
            <div v-for="(account, index) in accounts" :key="index" class="auth-account">
              <label :for="`${id}-user-${index}`"><span>{{ t('用户名') }}</span><input :id="`${id}-user-${index}`" v-model="account.username" autocomplete="off" autocapitalize="none" spellcheck="false" :disabled="blocked" required @input="error = ''"></label>
              <label :for="`${id}-pass-${index}`"><span>{{ t('密码') }}</span><input :id="`${id}-pass-${index}`" v-model="account.password" :type="passwordsVisible ? 'text' : 'password'" autocomplete="new-password" :disabled="blocked" required @input="error = ''"></label>
              <button class="ghost" type="button" :disabled="blocked" :aria-label="`${t('删除账号')} ${index + 1}`" @click="accounts.splice(index, 1); error = ''">{{ t('删除') }}</button>
            </div>
          </div>
          <div class="auth-actions"><button class="ghost" type="button" :disabled="blocked" @click="addAccount">{{ t('添加账号') }}</button><button class="ghost" type="button" :disabled="blocked" :aria-pressed="passwordsVisible" @click="passwordsVisible = !passwordsVisible">{{ t(passwordsVisible ? '隐藏密码' : '显示密码') }}</button></div>
        </template>
        <label class="auth-prefixes" :for="`${id}-prefixes`"><span>{{ t('免认证网段') }}</span><textarea :id="`${id}-prefixes`" v-model="prefixes" rows="3" spellcheck="false" :disabled="blocked" placeholder="127.0.0.0/8&#10;::1/128" @input="error = ''" /></label>
        <p class="field-note">{{ t('每行一个 IPv4 或 IPv6 CIDR，留空表示所有连接均需认证。127.0.0.0/8 与 ::1/128 可用于本机免认证。') }}</p>
      </template>
      <p v-if="error" class="error-text" role="alert">{{ t(error) }}</p>
    </form>
  </BaseModal>
</template>

<style scoped>
.auth-setting { grid-column: 1 / -1; cursor: default; }
.auth-setting small { display: block; color: var(--muted); }
.auth-setting > button { flex: none; white-space: nowrap; }
.auth-status { margin-left: 6px; color: var(--muted); font-size: 10px; font-weight: 400; }
.auth-toggle { display: flex; align-items: center; justify-content: space-between; margin: 16px 0; }
.auth-account-list { display: grid; gap: 12px; }
.auth-account { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto; align-items: end; gap: 10px; }
.auth-account label, .auth-prefixes { display: grid; gap: 6px; font-size: 12px; min-width: 0; }
.auth-account input, .auth-prefixes textarea { width: 100%; min-width: 0; }
.auth-account > button { height: 36px; }
.auth-actions { display: flex; gap: 10px; margin: 12px 0 18px; }
.auth-prefixes textarea { resize: vertical; }
.field-note { margin: 10px 0; color: var(--muted); font-size: 11px; line-height: 1.6; }
:global(.modal-card.proxy-auth-modal) { width: min(580px, 100%); padding: 16px; max-height: 85vh; overflow-y: auto; }
@media (max-width: 480px) { .auth-account { grid-template-columns: minmax(0, 1fr) auto; } .auth-account label { grid-column: 1; } .auth-account > button { grid-column: 2; grid-row: 1 / span 2; } }
</style>
