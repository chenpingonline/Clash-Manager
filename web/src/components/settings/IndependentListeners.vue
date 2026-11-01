<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import BaseModal from '@/components/BaseModal.vue'
import AppSelect from '@/components/AppSelect.vue'
import PolicySelect from '@/components/PolicySelect.vue'
import HelpPopover from '@/components/HelpPopover.vue'
import { RotateCcw } from 'lucide-vue-next'
import { api, errorMessage, jsonRequest } from '@/services/api'
import { notify } from '@/services/toast'
import { t } from '@/services/i18n'
import { availableListenerName, availableListenerPort, copyListener, listenerID } from '@/services/independent-listeners'
import type { IndependentListener, IndependentListenerStatus, ProfileItem } from '@/types/api'

const props = defineProps<{ busy: boolean }>()
const open = ref(false), loading = ref(false), saving = ref(false), loaded = ref(false), error = ref('')
const entries = ref<IndependentListener[]>([]), targets = ref<string[]>([]), reserved = ref<number[]>([])
const enabled = ref(true), current = ref(false), targetGroups = ref<string[]>([]), profiles = ref<ProfileItem[]>([]), profileId = ref('')
const unassigned = ref<IndependentListener[]>([]), assignUnassigned = ref(false)
const search = ref(''), selected = ref<string[]>([]), editor = ref<IndependentListener | null>(null), addressMode = ref('local')
const accountsOpen = ref(false)
const initial = ref(''), pendingSwitch = ref<string | null>(null), pendingClose = ref(false), pendingDelete = ref<string[]>([])
const trigger = ref<HTMLButtonElement>(), editorPanel = ref<HTMLElement>()
const blocked = computed(() => loading.value || saving.value || props.busy)
const fingerprint = () => JSON.stringify({ enabled: enabled.value, entries: entries.value, assignUnassigned: assignUnassigned.value })
const dirty = computed(() => loaded.value && fingerprint() !== initial.value)
const filtered = computed(() => entries.value.filter(entry => `${entry.name} ${entry.proxy} ${entry.port} ${entry.listen}`.toLowerCase().includes(search.value.toLowerCase())))
// Keep a draft visible in its original row even if its edited fields no longer
// match the search. A new draft is appended without committing it to the list.
const rows = computed(() => {
  const draft = editor.value
  const result = entries.value.filter(entry => entry.id === draft?.id || filtered.value.some(item => item.id === entry.id))
  if (draft && !entries.value.some(entry => entry.id === draft.id)) result.push(draft)
  return result
})
const rowCount = computed(() => entries.value.length + (editor.value && !entries.value.some(entry => entry.id === editor.value?.id) ? 1 : 0))
function setEditorRow(node: unknown) { editorPanel.value = node instanceof HTMLElement ? node : undefined }
function cancelEdit() { editor.value = null; accountsOpen.value = false; error.value = '' }
const allSelected = computed(() => filtered.value.length > 0 && filtered.value.every(entry => selected.value.includes(entry.id)))
const profileOptions = computed(() => [ ...(!profiles.value.some(item => item.current) ? [{ value: '', label: t('当前运行配置') }] : []), ...profiles.value.map(item => ({ value: item.id, label: item.name + (item.current ? ` · ${t('当前应用')}` : '') })) ])
const targetKinds = computed(() => Object.fromEntries(targets.value.map(name => [name, name === 'DIRECT' || name === 'REJECT' ? 'builtin' : targetGroups.value.includes(name) ? 'group' : 'node'])) as Record<string, 'builtin' | 'group' | 'node'>)
const connectionAddress = computed(() => {
  if (!editor.value) return ''
  const host = addressMode.value === 'lan' ? t('NAS 局域网 IP') : editor.value.listen
  return `${host.includes(':') ? `[${host}]` : host}:${editor.value.port}`
})
const endpoint = () => `/api/independent-listeners?profileId=${encodeURIComponent(profileId.value)}`
function apply(status: IndependentListenerStatus) {
  entries.value = (status.entries || []).map(copyListener)
  enabled.value = status.enabled !== false; current.value = status.current === true
  targets.value = status.targets || []; targetGroups.value = status.targetGroups || []; reserved.value = status.reservedPorts || []
  unassigned.value = (status.unassigned || []).map(copyListener); assignUnassigned.value = false
  selected.value = []; editor.value = null; search.value = ''; initial.value = fingerprint()
}
async function load() {
  loading.value = true; loaded.value = false; error.value = ''
  try { apply(await api<IndependentListenerStatus>(endpoint())); loaded.value = true }
  catch (cause) { error.value = errorMessage(cause) }
  finally { loading.value = false }
}
async function show() {
  if (props.busy) return
  open.value = true; loading.value = true; loaded.value = false; error.value = ''; pendingClose.value = false; pendingSwitch.value = null; pendingDelete.value = []
  try {
    const result = await api<{ items: ProfileItem[]; current?: string | null }>('/api/profiles')
    profiles.value = result.items || []; profileId.value = result.current || ''
    await load()
  } catch (cause) { error.value = errorMessage(cause) }
  finally { loading.value = false }
}
async function close(discard = false) {
  if (blocked.value) return
  if (!discard && (dirty.value || editor.value)) { pendingClose.value = true; return }
  open.value = false; entries.value = []; editor.value = null; loaded.value = false
  await nextTick(); trigger.value?.focus()
}
async function switchProfile(id: string, discard = false) {
  if (id === profileId.value) return
  if (!discard && (dirty.value || editor.value)) { pendingSwitch.value = id; return }
  profileId.value = id; pendingSwitch.value = null; pendingDelete.value = []; await load()
}
async function edit(entry: IndependentListener) {
  editor.value = copyListener(entry)
  addressMode.value = entry.listen === '127.0.0.1' ? 'local' : entry.listen === '0.0.0.0' ? 'lan' : 'custom'
  error.value = ''; pendingDelete.value = []; accountsOpen.value = false
  await nextTick(); editorPanel.value?.scrollIntoView({ block: 'nearest' }); editorPanel.value?.querySelector<HTMLInputElement>('[data-listener-name]')?.focus()
}
function setAddress(mode: string) {
  addressMode.value = mode
  if (editor.value) editor.value.listen = mode === 'local' ? '127.0.0.1' : mode === 'lan' ? '0.0.0.0' : ''
}
function chooseTarget(value: string) {
  if (!editor.value) return
  editor.value.proxy = value
  if (!editor.value.name.trim()) editor.value.name = availableListenerName(value, entries.value.filter(item => item.id !== editor.value?.id))
}
function add() {
  const port = availableListenerPort(entries.value, reserved.value)
  if (port === null) { error.value = '没有可用端口'; return }
  void edit({ id: listenerID(), name: '', type: 'mixed', listen: '127.0.0.1', port, proxy: '', enabled: true, udp: true, users: [] })
}
function duplicate(entry: IndependentListener) {
  const port = availableListenerPort(entries.value, reserved.value, entry.port + 1) ?? availableListenerPort(entries.value, reserved.value)
  if (port === null) { error.value = '没有可用端口'; return }
  void edit({ ...copyListener(entry), id: listenerID(), name: availableListenerName(entry.name, entries.value), port, enabled: false })
}
function finishEdit() {
  if (!editor.value) return
  const entry = copyListener(editor.value)
  entry.name = entry.name.trim()
  if (!entry.proxy || !entry.name) { error.value = '请选择目标并填写入口名称'; return }
  if (entries.value.some(item => item.id !== entry.id && item.name === entry.name)) { error.value = '入口名称已存在'; return }
  const users = new Set<string>()
  for (const user of entry.users) {
    if (!user.username.trim() || !user.password || users.has(user.username)) {
      error.value = '独立代理入口认证账户无效或重复'; accountsOpen.value = true; return
    }
    users.add(user.username)
  }
  const index = entries.value.findIndex(item => item.id === entry.id)
  if (index >= 0) entries.value[index] = entry
  else entries.value.push(entry)
  cancelEdit()
}
function toggleSelected(id: string) {
  selected.value = selected.value.includes(id) ? selected.value.filter(item => item !== id) : [...selected.value, id]
}
function selectAll() {
  const visible = filtered.value.map(item => item.id)
  selected.value = allSelected.value ? selected.value.filter(id => !visible.includes(id)) : [...new Set([...selected.value, ...visible])]
}
function bulkEnable(value: boolean) { for (const entry of entries.value) if (selected.value.includes(entry.id)) entry.enabled = value }
function removeSelected() {
  entries.value = entries.value.filter(entry => !pendingDelete.value.includes(entry.id))
  selected.value = selected.value.filter(id => !pendingDelete.value.includes(id)); pendingDelete.value = []
}
function assignLegacy() {
  if (entries.value.length + unassigned.value.length > 64) { error.value = '独立代理入口最多 64 个'; return }
  for (const entry of unassigned.value) entries.value.push({ ...copyListener(entry), id: entries.value.some(item => item.id === entry.id) ? listenerID() : entry.id, name: availableListenerName(entry.name, entries.value), enabled: false })
  assignUnassigned.value = true
}
function status(entry: IndependentListener) {
  if (!enabled.value) return t('总开关已关闭')
  if (!entry.enabled) return t('已停用')
  if (!targets.value.includes(entry.proxy)) return t('目标失效')
  return t('就绪')
}
async function save() {
  if (blocked.value || !loaded.value || editor.value) return
  saving.value = true; error.value = ''
  try {
    await api(endpoint(), jsonRequest('PUT', { entries: entries.value, enabled: enabled.value, assignUnassigned: assignUnassigned.value }))
    notify(current.value ? '独立代理入口已保存并应用' : '独立代理入口已保存，下次应用该配置时生效')
    await load()
  } catch (cause) { error.value = errorMessage(cause) }
  finally { saving.value = false }
}
</script>
<template>
  <div class="setting-toggle independent-setting">
    <div><strong>{{ t('独立代理入口') }}</strong><small>{{ t('按配置管理端口、目标和独立认证') }}</small></div>
    <button ref="trigger" class="ghost" type="button" :disabled="busy" aria-haspopup="dialog" :aria-expanded="open" @click="show">{{ t('管理入口') }}</button>
  </div>
  <BaseModal :open="open" :title="t('独立代理入口')" :show-header="false" :closable="!blocked" card-class="independent-listeners-modal" @close="close()">
    <form class="listener-form" @submit.prevent="finishEdit">
      <div class="settings-modal-header"><div class="listener-title"><h3>{{ t('独立代理入口') }}</h3><HelpPopover label="独立代理入口" :teleport="false" @keydown.esc.stop><span>{{ t('总开关仅控制应用创建的入口，订阅自带入口保持不变。') }}</span><span>{{ t('入口按目标名称跟随本配置更新；目标消失时暂停，同名目标恢复后自动恢复。') }}</span></HelpPopover></div><div class="settings-modal-header-actions"><span v-if="editor || dirty" class="hint">{{ t(editor ? '编辑中' : '待保存') }}</span><button class="ghost" type="button" :disabled="blocked" @click="close()">{{ t('关闭') }}</button><button type="button" :disabled="blocked || !loaded || !!editor || !dirty" @click="save">{{ t(saving ? '保存中…' : current ? '保存并应用' : '保存') }}</button></div></div>
      <p v-if="error" class="error-text" role="alert">{{ t(error) }}</p>
      <div v-if="pendingClose || pendingSwitch !== null" class="listener-confirm" role="alert">
        <span>{{ t('有未保存的更改，是否放弃？') }}</span><button class="ghost" type="button" @click="pendingClose = false; pendingSwitch = null">{{ t('继续编辑') }}</button><button class="ghost danger" type="button" @click="pendingClose ? close(true) : switchProfile(pendingSwitch!, true)">{{ t('放弃更改') }}</button>
      </div>
      <div class="listener-scroll">
        <div class="listener-toolbar"><label class="profile-field"><span>{{ t('配置') }}</span><AppSelect :model-value="profileId" :options="profileOptions" :disabled="blocked" :label="t('入口所属配置')" @update:model-value="switchProfile" /></label><label class="listener-check"><input v-model="enabled" type="checkbox" :disabled="blocked || !loaded || !!editor">{{ t('启用本配置的独立代理入口') }}</label></div>
        <p class="field-note">{{ t(current ? '当前配置：保存后立即校验并应用。' : '未应用配置：保存不影响当前运行，下次应用时生效。') }} </p>
        <p v-if="loading" role="status">{{ t('正在读取…') }}</p>
        <template v-if="loaded">
          <div v-if="unassigned.length && !assignUnassigned" class="listener-confirm"><span>{{ t('发现旧版待分配入口，添加到本配置后默认停用。') }}</span><button class="ghost" type="button" :disabled="blocked || !!editor" @click="assignLegacy">{{ t('添加到本配置') }}</button></div>
          <div class="listener-toolbar"><input v-model="search" :disabled="blocked || !!editor" type="search" :placeholder="t('搜索名称、目标、地址或端口')" :aria-label="t('搜索入口')"><span class="hint">{{ t('共 {arg0} 个入口', { arg0: rowCount }) }}</span><button class="ghost" type="button" :disabled="blocked || !!editor || entries.length >= 64" @click="add">{{ t('添加入口') }}</button></div>
          <div v-if="selected.length" class="listener-toolbar bulk-actions"><span>{{ t('已选择 {arg0} 个入口', { arg0: selected.length }) }}</span><button class="ghost" type="button" :disabled="blocked || !!editor" @click="bulkEnable(true)">{{ t('批量启用') }}</button><button class="ghost" type="button" :disabled="blocked || !!editor" @click="bulkEnable(false)">{{ t('批量停用') }}</button><button class="ghost danger" type="button" :disabled="blocked || !!editor" @click="pendingDelete = [...selected]">{{ t('批量删除') }}</button></div>
          <div v-if="pendingDelete.length" class="listener-confirm" role="alert"><span>{{ t('删除所选入口？保存后生效。') }}</span><button class="ghost" type="button" @click="pendingDelete = []">{{ t('取消') }}</button><button class="ghost danger" type="button" @click="removeSelected">{{ t('确认删除') }}</button></div>
          <div class="listener-table-wrap">
            <table class="listener-table">
              <colgroup><col class="col-select"><col class="col-enabled"><col class="col-name"><col class="col-protocol"><col class="col-address"><col class="col-port"><col class="col-target"><col class="col-udp"><col class="col-status"><col class="col-actions"></colgroup>
              <thead><tr>
                <th><input type="checkbox" :checked="allSelected" :disabled="!filtered.length || blocked || !!editor" :aria-label="t('选择全部可见入口')" @change="selectAll"></th>
                <th>{{ t('启用') }}</th><th>{{ t('入口名称') }}</th><th>{{ t('协议') }}</th><th>{{ t('监听地址') }}</th><th>{{ t('监听端口') }}</th><th>{{ t('目标节点或策略组') }}</th><th>UDP</th><th>{{ t('状态') }}</th><th>{{ t('操作') }}</th>
              </tr></thead>
              <tbody>
                <template v-for="entry in rows" :key="entry.id">
                  <tr v-if="editor && editor.id === entry.id" :ref="setEditorRow" class="listener-edit-row" :aria-label="t('编辑入口')">
                    <td><input type="checkbox" disabled :aria-label="t('选择入口 {arg0}', { arg0: editor.name || t('新入口') })"></td>
                    <td><input v-model="editor.enabled" type="checkbox" :disabled="blocked" :aria-label="t('启用此入口')"></td>
                    <td>
                      <div class="name-input">
                        <input v-model="editor.name" data-listener-name required maxlength="128" autocomplete="off" :disabled="blocked" :aria-label="t('入口名称')" :placeholder="t('入口名称')">
                        <button class="ghost name-fill" type="button" :disabled="blocked || !editor.proxy" :title="t('使用目标名称')" :aria-label="t('使用目标名称')" @click="editor.name = availableListenerName(editor.proxy, entries.filter(item => item.id !== editor?.id))"><RotateCcw :size="14" aria-hidden="true" /></button>
                      </div>
                    </td>
                    <td><AppSelect v-model="editor.type" :label="t('协议')" :disabled="blocked" :options="[{ value: 'mixed', label: 'Mixed' }, { value: 'socks', label: 'SOCKS5' }, { value: 'http', label: 'HTTP' }]" /></td>
                    <td>
                      <div class="address-input">
                        <AppSelect :model-value="addressMode" :label="t('监听方式')" :disabled="blocked" :options="[{ value: 'local', label: t('仅本机') }, { value: 'lan', label: t('所有 IPv4 网卡') }, { value: 'custom', label: t('指定地址') }]" @update:model-value="setAddress" />
                        <input v-if="addressMode === 'custom'" v-model="editor.listen" required placeholder="192.168.1.88" autocomplete="off" :disabled="blocked" :aria-label="t('监听地址')">
                        <HelpPopover label="监听地址" :teleport="false" @keydown.esc.stop>
                          <span>{{ t(addressMode === 'local' ? '仅 NAS 本机可连接：127.0.0.1' : addressMode === 'lan' ? '监听所有 IPv4 网卡，客户端使用 NAS 的局域网 IP 和此端口连接。' : '填写网卡的实际 IP，客户端使用该 IP 和此端口连接。') }}</span>
                          <span>{{ t('连接地址：{arg0}', { arg0: connectionAddress }) }}</span>
                        </HelpPopover>
                      </div>
                    </td>
                    <td><input v-model.number="editor.port" required type="number" min="1" max="65535" step="1" :disabled="blocked" :aria-label="t('监听端口')"></td>
                    <td class="listener-target"><PolicySelect :model-value="editor.proxy" :options="targets" :option-kinds="targetKinds" :disabled="blocked" :hide-label="true" label="目标节点或策略组" placeholder="请选择" @update:model-value="chooseTarget" /></td>
                    <td><input type="checkbox" :disabled="blocked || editor.type === 'http'" :checked="editor.type !== 'http' && editor.udp" @change="editor.udp = !editor.udp" :aria-label="t('启用 UDP')"></td>
                    <td class="hint">{{ t('编辑中') }}</td>
                    <td><div class="row-actions edit-actions">
                      <button type="submit" :disabled="blocked">{{ t('完成') }}</button>
                      <button class="ghost" type="button" :disabled="blocked" @click="cancelEdit">{{ t('取消') }}</button>
                      <button class="ghost" type="button" :disabled="blocked" :aria-expanded="accountsOpen" :aria-controls="`listener-users-${entry.id}`" @click="accountsOpen = !accountsOpen">{{ t('认证') }}<span v-if="editor.users.length"> · {{ editor.users.length }}</span></button>
                    </div></td>
                  </tr>
                  <tr v-else>
                    <td><input type="checkbox" :checked="selected.includes(entry.id)" :disabled="blocked || !!editor" :aria-label="t('选择入口 {arg0}', { arg0: entry.name })" @change="toggleSelected(entry.id)"></td>
                    <td><input v-model="entry.enabled" type="checkbox" :disabled="blocked || !!editor" :aria-label="t('启用入口 {arg0}', { arg0: entry.name })"></td>
                    <td class="listener-name" :title="entry.name">{{ entry.name }}</td><td>{{ entry.type === 'socks' ? 'SOCKS5' : entry.type === 'http' ? 'HTTP' : 'Mixed' }}</td>
                    <td class="listener-name" :title="entry.listen">{{ entry.listen }}</td><td>{{ entry.port }}</td><td class="listener-name" :title="entry.proxy">{{ entry.proxy }}</td>
                    <td>{{ t(entry.type !== 'http' && entry.udp ? '开启' : '关闭') }}</td>
                    <td :class="entry.enabled && enabled && !targets.includes(entry.proxy) ? 'warn-text' : 'hint'">{{ status(entry) }}</td>
                    <td><div class="row-actions">
                      <button class="ghost" type="button" :disabled="blocked || !!editor" :aria-label="t('编辑入口 {arg0}', { arg0: entry.name })" @click="edit(entry)">{{ t('编辑') }}</button>
                      <button class="ghost" type="button" :disabled="blocked || !!editor || entries.length >= 64" :aria-label="t('复制入口 {arg0}', { arg0: entry.name })" @click="duplicate(entry)">{{ t('复制') }}</button>
                      <button class="ghost danger" type="button" :disabled="blocked || !!editor" :aria-label="t('删除入口 {arg0}', { arg0: entry.name })" @click="pendingDelete = [entry.id]">{{ t('删除') }}</button>
                    </div></td>
                  </tr>
                  <tr v-if="editor && editor.id === entry.id && accountsOpen" class="listener-auth-row">
                    <td colspan="10"><fieldset :id="`listener-users-${entry.id}`" class="listener-auth" :disabled="blocked">
                      <legend>{{ t('认证') }}</legend>
                      <p class="field-note">{{ t('每个入口独立认证；不添加账户表示免认证。') }}</p>
                      <div v-for="(user, index) in editor.users" :key="index" class="listener-users">
                        <label><span>{{ t('用户名') }}</span><input v-model="user.username" required autocomplete="off" maxlength="128"></label>
                        <label><span>{{ t('密码') }}</span><input v-model="user.password" required type="password" autocomplete="new-password" maxlength="256"></label>
                        <button class="ghost danger" type="button" :aria-label="t('删除账号')" @click="editor.users.splice(index, 1)">{{ t('删除') }}</button>
                      </div>
                      <button class="ghost" type="button" :disabled="editor.users.length >= 16" @click="editor.users.push({ username: '', password: '' })">{{ t('添加账号') }}</button>
                    </fieldset></td>
                  </tr>
                </template>
                <tr v-if="!rows.length"><td colspan="10" class="empty-state">{{ t(entries.length ? '没有匹配的入口' : '尚未添加独立代理入口') }}</td></tr>
              </tbody>
            </table>
          </div>
        </template>
      </div>
    </form>
  </BaseModal>
</template>
<style scoped>
.independent-setting { grid-column: 1 / -1; }
.independent-setting small { display: block; color: var(--muted); }
.independent-setting > button { flex: none; white-space: nowrap; }
.listener-form { display: flex; flex-direction: column; min-height: 0; max-height: calc(88vh - 32px); }
.listener-title { display: flex; align-items: center; gap: 8px; }
.listener-title h3 { margin: 0; }
.listener-scroll { min-height: 0; overflow-y: auto; padding: 0 3px 3px; }
.listener-toolbar, .listener-check, .listener-confirm, .row-actions, .name-input, .address-input { display: flex; align-items: center; gap: 8px; }
.listener-toolbar { flex-wrap: wrap; margin: 12px 0; gap: 10px 16px; }
.listener-toolbar > input[type=search] { flex: 1; min-width: 160px; width: auto; }
.profile-field { display: flex; align-items: center; gap: 8px; flex: 1; min-width: 180px; }
.profile-field :deep(.app-select) { flex: 1; }
.listener-check { white-space: nowrap; }
.listener-check input, .listener-table input[type=checkbox] { width: 14px; height: 14px; margin: 0; flex: none; }
.field-note { color: var(--muted); font-size: 12px; line-height: 1.6; margin: 8px 0; }
.listener-confirm { flex-wrap: wrap; padding: 10px; border: 1px solid var(--line); border-radius: 8px; margin: 8px 0; }
.listener-confirm > span { flex: 1; min-width: 160px; }
.listener-table-wrap { overflow-x: auto; border: 1px solid var(--line); border-radius: 8px; margin-bottom: 2px; }
.listener-table { width: 100%; min-width: 1080px; table-layout: fixed; border-collapse: collapse; text-align: left; font-size: 12px; }
.listener-table .col-select { width: 30px; }
.listener-table .col-enabled { width: 42px; }
.listener-table .col-name { width: 160px; }
.listener-table .col-protocol { width: 104px; }
.listener-table .col-address { width: 164px; }
.listener-table .col-port { width: 80px; }
.listener-table .col-target { width: 200px; }
.listener-table .col-udp { width: 42px; }
.listener-table .col-status { width: 64px; }
.listener-table .col-actions { width: 160px; }
.listener-table th, .listener-table td { padding: 9px 7px; border-bottom: 1px solid var(--line); white-space: nowrap; }
.listener-table tr:last-child td { border-bottom: 0; }
.listener-name { overflow: hidden; text-overflow: ellipsis; }
.listener-table .empty-state { text-align: center; color: var(--muted); padding: 24px; }
.row-actions { gap: 5px; flex-wrap: wrap; }
.row-actions button { padding: 4px 6px; font-size: 12px; white-space: nowrap; }
.listener-edit-row, .listener-auth-row { background: rgba(70,130,235,.06); }
.listener-edit-row td { vertical-align: middle; }
.listener-edit-row input:not([type=checkbox]), .listener-edit-row :deep(.app-select), .listener-edit-row :deep(.policy-select-trigger) { width: 100%; min-width: 0; min-height: 30px; height: 30px; padding: 4px 6px; font-size: 12px; }
.listener-edit-row :deep(.policy-select) { min-width: 0; }
.name-input { gap: 4px; }
.name-input input { flex: 1; }
.name-fill { flex: none; display: grid; place-items: center; padding: 4px; }
.address-input { display: grid; grid-template-columns: minmax(0,1fr) auto; gap: 5px; }
.address-input > input { grid-column: 1; }
.address-input > :deep(.help-popover) { grid-column: 2; grid-row: 1; }
.listener-auth-row td { padding: 0; white-space: normal; }
.listener-auth { position: sticky; left: 0; margin: 0; border: 0; padding: 12px 16px; max-width: 840px; min-width: 0; }
.listener-auth legend { float: left; margin-right: 12px; font-size: 12px; font-weight: 600; }
.listener-auth .field-note { margin-top: 0; }
.listener-users { display: grid; grid-template-columns: minmax(0,1fr) minmax(0,1fr) auto; align-items: center; gap: 12px; margin: 8px 0; }
.listener-users label { display: flex; align-items: center; gap: 8px; font-size: 12px; min-width: 0; }
.listener-users label > span { flex: none; white-space: nowrap; }
.listener-users input { flex: 1; width: 100%; min-width: 0; }
.listener-auth input, .listener-auth button { min-height: 30px; height: 30px; padding: 4px 8px; font-size: 12px; }
.listener-auth button { white-space: nowrap; }
.danger { color: var(--danger, #d64c4c); }
:global(.modal-card.independent-listeners-modal) { width: min(1240px,100%); max-height: 88vh; overflow: hidden; padding: 16px; }
@media (max-width:600px) {
 .settings-modal-header { flex-wrap: wrap; gap: 8px; }
 .profile-field { flex-basis: 100%; }
 .listener-auth { max-width: calc(100vw - 72px); }
 .listener-users { grid-template-columns: minmax(0,1fr); }
}
</style>
