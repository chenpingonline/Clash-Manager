<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import BaseModal from '@/components/BaseModal.vue'
import { api, errorMessage, jsonRequest } from '@/services/api'
import { notify } from '@/services/toast'
import { t } from '@/services/i18n'
import type { IndependentListener, IndependentListenerStatus, NetworkSettingsResponse } from '@/types/api'
const props = defineProps<{ busy: boolean }>()
const open = ref(false), loading = ref(false), saving = ref(false), loaded = ref(false), error = ref('')
const entries = ref<IndependentListener[]>([]), targets = ref<string[]>([]), missing = ref<string[]>([])
const trigger = ref<HTMLButtonElement>()
const blocked = computed(() => loading.value || saving.value)
function apply(status?: IndependentListenerStatus) {
  entries.value = (status?.entries || []).map(item => ({ ...item, users: (item.users || []).map(user => ({ ...user })) }))
  targets.value = status?.targets || []; missing.value = status?.missingTargets || []
}
async function show() {
  if (props.busy) return
  open.value = true; loading.value = true; loaded.value = false; error.value = ''
  try {
    const result = await api<NetworkSettingsResponse>('/api/network/settings')
    if (!result.settings?.independentListeners) throw new Error('无法读取独立代理入口')
    apply(result.settings.independentListeners); loaded.value = true
  } catch (cause) { error.value = errorMessage(cause) }
  finally { loading.value = false }
}
async function close() {
  if (blocked.value) return
  open.value = false; entries.value = []; loaded.value = false
  await nextTick(); trigger.value?.focus()
}
function add() {
  const id = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
  entries.value.push({ id, name: '', type: 'mixed', listen: '127.0.0.1', port: 1080, proxy: '', enabled: true, users: [] })
}
async function save() {
  if (blocked.value || props.busy || !loaded.value) return
  saving.value = true; error.value = ''
  try {
    const result = await api<NetworkSettingsResponse>('/api/network/settings', jsonRequest('PUT', { independentListeners: entries.value }))
    apply(result.settings?.independentListeners)
    notify(missing.value.length ? '独立入口已保存，部分目标失效，相关入口已停用' : '独立代理入口已保存并应用', missing.value.length > 0)
    saving.value = false; await close()
  } catch (cause) { error.value = errorMessage(cause) }
  finally { saving.value = false }
}
</script>
<template>
  <div class="setting-toggle independent-setting">
    <div><strong>{{ t('独立代理入口') }}</strong><small>{{ t('为不同端口指定节点或策略组，订阅更新后保留') }}</small></div>
    <button ref="trigger" class="ghost" type="button" :disabled="busy" aria-haspopup="dialog" :aria-expanded="open" @click="show">{{ t('管理入口') }}</button>
  </div>
  <BaseModal :open="open" :title="t('独立代理入口')" :show-header="false" :closable="!blocked" card-class="independent-listeners-modal" @close="close">
    <form class="listener-form" @submit.prevent="save">
      <div class="settings-modal-header"><h3>{{ t('独立代理入口') }}</h3><div class="settings-modal-header-actions"><button class="ghost" type="button" :disabled="blocked" @click="close">{{ t('取消') }}</button><button type="submit" :disabled="blocked || busy || !loaded">{{ t(saving ? '保存中…' : '保存') }}</button></div></div>
      <p v-if="error" class="error-text" role="alert">{{ t(error) }}</p>
      <div class="listener-scroll">
      <p class="field-note">{{ t('每个入口使用独立认证；不添加账户表示免认证。默认仅本机可访问，局域网访问请明确填写监听地址。') }}</p>
      <p class="field-note">{{ t('目标按名称跟随订阅更新；目标消失时停止该入口，同名目标恢复后自动恢复。订阅自带入口保持不变。') }}</p>
      <p v-if="loading" role="status">{{ t('正在读取…') }}</p>
      <template v-if="loaded">
        <p v-if="!entries.length" class="hint">{{ t('尚未添加独立代理入口') }}</p>
        <fieldset v-for="(entry, index) in entries" :key="entry.id" class="listener-entry" :disabled="blocked">
          <legend>{{ entry.name || t('新入口') }}</legend>
          <div class="listener-heading"><label class="listener-enabled"><input v-model="entry.enabled" type="checkbox">{{ t('启用') }}</label><button class="ghost" type="button" :aria-label="t('删除入口 {arg0}', { arg0: entry.name || index + 1 })" @click="entries.splice(index, 1)">{{ t('删除') }}</button></div>
          <p v-if="entry.enabled && (!targets.includes(entry.proxy) || missing.includes(entry.id))" class="warn-text" role="status">{{ t('目标失效或尚未选择，此入口不会监听端口') }}</p>
          <div class="listener-grid">
            <label>{{ t('入口名称') }}<input v-model="entry.name" required maxlength="128" autocomplete="off"></label>
            <label>{{ t('协议') }}<select v-model="entry.type"><option value="mixed">Mixed (HTTP / SOCKS5)</option><option value="socks">SOCKS5</option><option value="http">HTTP</option></select></label>
            <label>{{ t('监听地址') }}<input v-model="entry.listen" required placeholder="127.0.0.1" autocomplete="off"></label>
            <label>{{ t('监听端口') }}<input v-model.number="entry.port" required type="number" min="1" max="65535" step="1"></label>
            <label class="listener-target">{{ t('目标节点或策略组') }}<select v-model="entry.proxy" required @change="missing = missing.filter(id => id !== entry.id)"><option disabled value="">{{ t('请选择') }}</option><option v-if="entry.proxy && !targets.includes(entry.proxy)" :value="entry.proxy">{{ entry.proxy }} ({{ t('目标失效') }})</option><option v-for="target in targets" :key="target" :value="target">{{ target }}</option></select></label>
          </div>
          <div v-for="(user, userIndex) in entry.users" :key="userIndex" class="listener-users">
            <label>{{ t('用户名') }}<input v-model="user.username" required autocomplete="off" maxlength="128"></label>
            <label>{{ t('密码') }}<input v-model="user.password" required type="password" autocomplete="new-password" maxlength="256"></label>
            <button class="ghost" type="button" :aria-label="t('删除账号')" @click="entry.users.splice(userIndex, 1)">{{ t('删除') }}</button>
          </div>
          <button class="ghost" type="button" :disabled="entry.users.length >= 16" @click="entry.users.push({username: '', password: ''})">{{ t('添加账号') }}</button>
        </fieldset>
        <button class="ghost" type="button" :disabled="blocked || entries.length >= 64" @click="add">{{ t('添加入口') }}</button>
      </template>
      </div>
    </form>
  </BaseModal>
</template>
<style scoped>
.independent-setting { grid-column: 1 / -1; }
.independent-setting small { display: block; color: var(--muted); }
.independent-setting > button { flex: none; white-space: nowrap; }
.field-note { font-size: 12px; color: var(--muted); line-height: 1.6; margin: 10px 0; }
.listener-entry { border: 1px solid var(--line); border-radius: 10px; padding: 12px; margin: 14px 0; min-width: 0; }
.listener-entry legend { font-size: 13px; overflow-wrap: anywhere; max-width: 100%; padding: 0 5px; }
.listener-heading, .listener-enabled { display: flex; align-items: center; gap: 8px; }
.listener-enabled { flex: none; white-space: nowrap; }
.listener-enabled input { width: 14px; height: 14px; flex: none; margin: 0; }
.listener-heading { justify-content: space-between; margin-bottom: 8px; }
.listener-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin-bottom: 12px; }
.listener-grid label, .listener-users label { display: grid; gap: 5px; font-size: 12px; min-width: 0; }
.listener-grid input, .listener-grid select, .listener-users input { width: 100%; min-width: 0; }
.listener-grid select { min-height: 36px; border: 1px solid var(--line); border-radius: 7px; background: var(--surface-inset); color: var(--text); padding: 0 8px; font: inherit; }
.listener-form { display: flex; flex-direction: column; min-height: 0; max-height: calc(85vh - 32px); }
.listener-scroll { min-height: 0; overflow-y: auto; padding: 0 3px 3px; }
.listener-target { grid-column: 1 / -1; }
.listener-users { display: grid; grid-template-columns: minmax(0,1fr) minmax(0,1fr) auto; align-items: end; gap: 8px; margin: 10px 0; }
:global(.modal-card.independent-listeners-modal) { width: min(660px,100%); max-height: 85vh; overflow: hidden; padding: 16px; }
@media (max-width:480px) { .listener-grid { grid-template-columns: minmax(0,1fr); } .listener-users { grid-template-columns:minmax(0,1fr); } }
</style>
