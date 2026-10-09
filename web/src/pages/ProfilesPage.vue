<script setup lang="ts">
import { t, getLocale } from '@/services/i18n'

import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import AsyncState from '@/components/AsyncState.vue'
import BaseModal from '@/components/BaseModal.vue'
import ProfileExtensionEditor from '@/components/ProfileExtensionEditor.vue'
import { api, errorMessage, isAbortError, jsonRequest } from '@/services/api'
import { formatBytes, formatTime, normalizeSubscriptionInfo } from '@/services/format'
import { streamProfileJob } from '@/services/profile-jobs'
import { notify } from '@/services/toast'
import type { LocalConfigCandidate, LocalDiscoveryResponse, LocalRuntime, ProfileExtensionKind, ProfileItem, ProfileJob, ProfilesResponse } from '@/types/api'

import { runtime } from '@/services/runtime'

type RemoteForm = { name: string; url: string; intervalMinutes: number; autoUpdate: boolean; autoApply: boolean }
const emptyForm = (): RemoteForm => ({ name: '', url: '', intervalMinutes: 360, autoUpdate: true, autoApply: false })
const loading = ref(true), error = ref(''), localLoading = ref(true), localError = ref(''), modal = ref<'remote' | 'file' | null>(null)
const items = ref<ProfileItem[]>([]), discovery = ref<LocalDiscoveryResponse>({}), editing = ref<ProfileItem | null>(null), busyId = ref('')
const profileJobs = reactive<Record<string, ProfileJob>>({})
const extensionProfile = ref<ProfileItem | null>(null), extensionKind = ref<ProfileExtensionKind | null>(null)
const extensionGlobal = ref(false)
const openEditMenuId = ref('')
const form = reactive<RemoteForm>(emptyForm()), fileName = ref('本地配置'), selectedFile = ref<File | null>(null)
const importedPaths = computed(() => new Set(items.value.map(item => item.sourcePath).filter(Boolean)))
let alive = true
const jobControllers = new Map<string, AbortController>()

function openRemote(item?: ProfileItem) {
  editing.value = item || null
  Object.assign(form, item ? { name: item.name, url: item.url || '', intervalMinutes: item.intervalMinutes ?? 360, autoUpdate: Boolean(item.autoUpdate), autoApply: Boolean(item.autoApply) } : emptyForm())
  modal.value = 'remote'
}
function toggleEditMenu(item: ProfileItem) {
  openEditMenuId.value = openEditMenuId.value === item.id ? '' : item.id
}
function closeEditMenu() { openEditMenuId.value = '' }
function handlePagePointerDown(event: PointerEvent) {
  if (!(event.target instanceof Element) || !event.target.closest('.profile-edit-menu')) closeEditMenu()
}
function handlePageKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') closeEditMenu()
}
function openExtension(item: ProfileItem, kind: ProfileExtensionKind) {
  closeEditMenu()
  extensionGlobal.value = false
  extensionProfile.value = item
  extensionKind.value = kind
}
function openGlobalExtension(kind: 'override' | 'script') {
  extensionGlobal.value = true
  extensionProfile.value = null
  extensionKind.value = kind
}
function closeExtension() {
  extensionGlobal.value = false
  extensionProfile.value = null
  extensionKind.value = null
}
function downloadText(item: ProfileItem) {
  const info = item.lastDownload
  if (!info) return ''
  const attempts = info.attempts || []
  const summary = attempts.map(a => a.skipped ? `${t(a.label)}: ${t('跳过')}` : a.status ? `${t(a.label)}: HTTP ${a.status}` : `${t(a.label)}: ${t(a.error || '失败')}`).join(getLocale() === 'en-US' ? '; ' : '；')
  const ok = Boolean(info.method && info.method !== 'failed' && ((Number(info.status) >= 200 && Number(info.status) < 300) || Number(info.status) === 304))
  return ok ? `${t('最近下载：')}${t(info.label || info.method)} · ${info.unchanged ? `${t('内容未变化 ·')} ` : `HTTP ${info.status} · `}${Number(info.durationMs || 0)} ms` : `${t('最近下载：')}${t(info.label || '更新失败')}${summary ? ` · ${summary}` : ''}`
}
function quota(item: ProfileItem) {
  const info = normalizeSubscriptionInfo(item.subscriptionInfo)
  if (!info) return null
  const used = info.upload + info.download, remain = info.total > 0 ? Math.max(0, info.total - used) : 0
  const percent = info.total > 0 ? Math.max(0, Math.min(100, used / info.total * 100)) : 0
  if (!info.expire) return { used, remain, total: info.total, percent, expire: '长期有效', className: '' }
  const ms = info.expire > 1e12 ? info.expire : info.expire * 1000, days = Math.ceil((ms - Date.now()) / 86_400_000)
  return { used, remain, total: info.total, percent, expire: days < 0 ? '已过期' : days === 0 ? '今天到期' : `${new Date(ms).toLocaleDateString(getLocale())} · 剩余 ${days} 天`, className: days < 0 ? 'bad' : days <= 7 ? 'warn' : '' }
}
function candidateState(item: LocalConfigCandidate) { return item.readable ? '可读取' : item.permissionDenied || item.exists ? '无读取权限' : '未找到' }
function runtimeTitle(runtime: LocalRuntime) {
  if (runtime.mode === 'managed') return runtime.running ? 'Manager 正在托管 Mihomo Core' : '已选择 Manager 托管模式'
  if (runtime.mode === 'external') return runtime.running ? '本机 Mihomo Core 正在运行' : '已选择本机 Core 模式'
  return runtime.running ? '已检测到 Mihomo Core' : '正在自动检测 Mihomo Core'
}
function runtimeDetail(runtime: LocalRuntime) {
  if (!runtime.running) return t(runtime.message || '当前未检测到运行中的 Core，请到设置页面检查启动状态')
  const identity = [runtime.pid ? `PID ${runtime.pid}` : '', runtime.binaryVersion ? `v${runtime.binaryVersion.replace(/^v/, '')}` : ''].filter(Boolean).join(' · ')
  return [identity, runtime.configPath ? t('配置 {arg0}', { arg0: runtime.configPath }) : runtime.binaryPath].filter(Boolean).join(' · ')
}
function runtimeTag(runtime: LocalRuntime) { return runtime.mode === 'managed' ? '托管模式' : runtime.mode === 'external' ? '本机 Core' : '自动检测' }

async function loadProfiles() {
  try { const data = await api<ProfilesResponse>('/api/profiles'); if (alive) { items.value = data.items || []; error.value = '' } }
  catch (cause) { if (alive) error.value = errorMessage(cause) }
  finally { if (alive) loading.value = false }
}
async function scan() {
  localLoading.value = true
  try { discovery.value = await api<LocalDiscoveryResponse>('/api/local-config/discover'); localError.value = '' }
  catch (cause) { localError.value = errorMessage(cause) }
  finally { localLoading.value = false }
}
async function saveRemote() {
  if (!form.name.trim()) return notify('请输入订阅名称', true)
  if (!form.url.trim()) return notify('请输入订阅 URL', true)
  if (!Number.isFinite(form.intervalMinutes) || form.intervalMinutes < 5) return notify('更新间隔不能小于 5 分钟', true)
  busyId.value = 'remote'
  try {
    const payload = { ...form, name: form.name.trim(), url: form.url.trim() }
    await api(editing.value ? `/api/profiles/${editing.value.id}` : '/api/profiles', jsonRequest(editing.value ? 'PATCH' : 'POST', payload))
    notify(editing.value ? '订阅设置已保存' : '订阅已添加'); modal.value = null; await loadProfiles()
  } catch (cause) { notify(errorMessage(cause), true) }
  finally { busyId.value = '' }
}
async function importFile() {
  if (!selectedFile.value) return notify('请选择文件', true)
  busyId.value = 'file'
  try { await api('/api/profiles/import', jsonRequest('POST', { name: fileName.value.trim() || '本地配置', content: await selectedFile.value.text() })); modal.value = null; notify('已导入'); await loadProfiles() }
  catch (cause) { notify(errorMessage(cause), true) }
  finally { busyId.value = '' }
}
async function importNas(candidate: LocalConfigCandidate, apply: boolean) {
  if (!candidate.token) return
  busyId.value = `local-${candidate.token}-${apply}`
  try { const result = await api<{ sourcePath?: string }>('/api/local-config/import', jsonRequest('POST', { token: candidate.token, apply })); notify(`${apply ? '已导入并应用' : '已导入'}：${result.sourcePath || candidate.path}`); await Promise.all([loadProfiles(), scan()]) }
  catch (cause) { notify(errorMessage(cause), true) }
  finally { busyId.value = '' }
}
async function runProfileJob(item: ProfileItem, operation: 'update' | 'activate') {
  busyId.value = `${operation}-${item.id}`
  try {
    let job = await api<ProfileJob>(`/api/profiles/${item.id}/${operation}`, { method: 'POST' })
    if (!job.jobId) throw new Error('未获取到后台任务')
    profileJobs[item.id] = job
    const controller = new AbortController()
    jobControllers.set(item.id, controller)
    job = await streamProfileJob(job.jobId, next => { if (alive) profileJobs[item.id] = next }, controller.signal)
    if (job.state === 'failed') {
      profileJobs[item.id] = { ...job, message: job.error ? `${job.message || '操作失败'}：${job.error}` : job.message }
      throw new Error(job.error || (operation === 'update' ? '订阅更新失败' : '配置应用失败'))
    }
    if (operation === 'update') {
      const dl = job.result?.lastDownload
      notify(dl?.unchanged ? `订阅没有变化 · ${Number(dl.durationMs || 0)} ms` : dl?.label ? `订阅更新完成 · ${dl.label} · ${Number(dl.durationMs || 0)} ms` : '订阅配置已安全更新')
    } else {
      notify(job.result?.unchanged ? '配置内容没有变化，已跳过重复应用' : `配置已应用并同步到 ${job.result?.target || '启动配置'} · ${Number(job.result?.durationMs || 0)} ms`)
    }
    window.setTimeout(() => { if (profileJobs[item.id]?.jobId === job.jobId) delete profileJobs[item.id] }, 1800)
  } catch (cause) { if (!isAbortError(cause)) notify(errorMessage(cause), true) }
  finally { jobControllers.delete(item.id); busyId.value = ''; await loadProfiles() }
}
function update(item: ProfileItem) { return runProfileJob(item, 'update') }
function activate(item: ProfileItem) { return runProfileJob(item, 'activate') }
async function remove(item: ProfileItem) {
  if (!confirm(t('确定删除这个配置吗？'))) return
  try { await api(`/api/profiles/${item.id}`, { method: 'DELETE' }); notify('已删除'); await loadProfiles() }
  catch (cause) { notify(errorMessage(cause), true) }
}
onMounted(() => {
  window.addEventListener('pointerdown', handlePagePointerDown)
  window.addEventListener('keydown', handlePageKeydown)
  return Promise.all([loadProfiles(), scan()])
})
onBeforeUnmount(() => {
  alive = false
  window.removeEventListener('pointerdown', handlePagePointerDown)
  window.removeEventListener('keydown', handlePageKeydown)
  jobControllers.forEach(controller => controller.abort())
  jobControllers.clear()
})
</script>

<template>
  <div class="card section"><div class="actions"><button class="small" @click="openRemote()">{{ t("添加订阅") }}</button><button class="ghost small" @click="modal = 'file'">{{ t("从当前电脑导入 YAML") }}</button></div><p class="profile-source-note">{{ t("订阅可包含节点、代理组和分流规则。软件默认不会额外添加代理组或分流规则；你设置的订阅增强和全局增强也会参与生成最终配置。") }}</p></div>
  <div class="card section">
    <div class="section-head"><div><h2>{{ t("配置列表") }}</h2></div></div>
    <AsyncState :loading="loading" :error="t(error)">
      <div v-if="items.length" class="profile-list">
        <div v-for="item in items" :key="item.id" class="profile" :class="{ current: item.current }">
          <div>
            <div class="profile-title-line">
              <div class="profile-name">{{ t(item.current ? '● ' : '') }}{{ item.name }}</div>
            </div>
            <div class="profile-meta">{{ t(item.type === 'remote' ? '远程订阅' : '本地配置') }} {{ t("· 更新：") }}{{ t(formatTime(item.updatedAt)) }}</div>
            <div v-if="item.lastError" class="profile-meta error-text">{{ t(item.lastError) }}</div>
            <div v-if="downloadText(item)" class="download-info" :class="item.lastDownload?.method === 'failed' ? 'bad' : 'good'">{{ downloadText(item) }}</div>
          </div>
          <div class="profile-details">
            <template v-if="item.type === 'remote'">
              <template v-for="q in [quota(item)]" :key="item.id">
                <div v-if="q" class="subscription-info">
                  <template v-if="q.total">
                    <div class="quota-line"><span>{{ t("已用") }} {{ t(formatBytes(q.used)) }}</span><span>{{ t("剩余") }} <strong>{{ t(formatBytes(q.remain)) }}</strong> / {{ t(formatBytes(q.total)) }}</span></div>
                    <div class="quota-track"><span :style="{ width: `${q.percent}%` }" /></div>
                  </template>
                  <div class="quota-expire" :class="q.className">{{ t(q.expire) }}</div>
                </div>
              </template>
            </template>
            <div v-else class="profile-url" :title="item.sourcePath || ''">{{ item.sourcePath || t('本地导入') }}</div>
          </div>
          <div class="actions">
            <button v-if="item.type === 'remote'" class="ghost small" :disabled="Boolean(busyId)" @click="update(item)">{{ t(busyId === `update-${item.id}` ? '处理中…' : '更新') }}</button>
            <button class="success small" :disabled="Boolean(busyId)" @click="activate(item)">{{ t(busyId === `activate-${item.id}` ? '应用中…' : '应用') }}</button>
            <div class="profile-edit-menu">
              <button type="button" class="ghost small profile-edit-trigger" :aria-label="t(`编辑 ${item.name}`)" aria-haspopup="menu" :aria-expanded="openEditMenuId === item.id" @click="toggleEditMenu(item)">
                <span>{{ t("编辑") }}</span><svg viewBox="0 0 16 16" aria-hidden="true"><path d="m5 6 3 3 3-3" /></svg>
              </button>
              <div v-if="openEditMenuId === item.id" class="profile-edit-popover" role="menu">
                <button type="button" role="menuitem" @click="openExtension(item, 'rules')">{{ t("编辑规则") }}</button>
                <button type="button" role="menuitem" @click="openExtension(item, 'proxies')">{{ t("编辑节点") }}</button>
                <button type="button" role="menuitem" @click="openExtension(item, 'groups')">{{ t("编辑代理组") }}</button>
                <button type="button" role="menuitem" @click="openExtension(item, 'override')">{{ t("扩展覆写配置") }}</button>
                <button type="button" role="menuitem" @click="openExtension(item, 'script')">{{ t("扩展脚本") }}</button>
                <button v-if="item.type === 'remote'" type="button" role="menuitem" class="profile-edit-secondary" @click="closeEditMenu(); openRemote(item)">{{ t("订阅信息") }}</button>
              </div>
            </div>
            <button class="danger small" :disabled="Boolean(busyId)" @click="remove(item)">{{ t("删除") }}</button>
          </div>
          <template v-for="job in [profileJobs[item.id]]" :key="job?.jobId || item.id">
            <div v-if="job" class="profile-operation-status profile-operation-row" :class="job.state" role="status" aria-live="polite">
              <span v-if="job.state === 'running'" class="profile-operation-spinner" />
              <span>{{ t(job.message) }}</span>
            </div>
          </template>
        </div>
      </div>
      <div v-else class="empty">{{ t("还没有配置") }}</div>
    </AsyncState>
    <div class="global-extension-grid">
      <div class="global-extension-card">
        <span class="global-extension-icon" aria-hidden="true">M</span>
        <span class="global-extension-copy"><strong>{{ t("全局扩展覆写配置") }}</strong><small>{{ t("应用到所有配置，先递归合并") }}</small></span>
        <span class="global-extension-actions"><span class="tag">Merge</span><button type="button" class="ghost small global-extension-edit" :aria-label="t('编辑全局扩展覆写配置')" @click="openGlobalExtension('override')">{{ t("编辑") }}</button></span>
      </div>
      <div class="global-extension-card">
        <span class="global-extension-icon script" aria-hidden="true">JS</span>
        <span class="global-extension-copy"><strong>{{ t("全局扩展脚本") }}</strong><small>{{ t("全局覆写后、单配置增强前执行") }}</small></span>
        <span class="global-extension-actions"><span class="tag">Script</span><button type="button" class="ghost small global-extension-edit" :aria-label="t('编辑全局扩展脚本')" @click="openGlobalExtension('script')">{{ t("编辑") }}</button></span>
      </div>
    </div>
  </div>
  <div class="card section local-discovery-card"><div class="section-head"><div><h2>{{ t("本机 Mihomo 配置") }}</h2><p>{{ t(runtime.platform === 'linux' ? '读取托管 Mihomo 配置和 APP_IMPORT_PATHS 指定的导入目录' : runtime.platform === 'docker' ? '读取容器内 Mihomo 配置和明确挂载的导入目录' : '自动读取当前 Mihomo 配置；用户文件仅从 fnOS 明确授权的目录读取') }}</p></div><button class="ghost small" @click="scan">{{ t("重新扫描") }}</button></div><AsyncState :loading="localLoading" :error="t(localError)"><div v-if="discovery.error" class="local-warning">{{ t("扫描失败：") }}{{ t(discovery.error) }}</div><div class="local-access-summary" :class="discovery.authorizedPaths?.length ? 'active' : 'warn'"><strong>{{ t(discovery.authorizedPaths?.length ? `已授权 ${discovery.authorizedPaths.length} 个文件夹` : '尚未授权用户文件夹') }}</strong><span v-if="discovery.authorizedPaths?.length"><span v-for="path in discovery.authorizedPaths" :key="path" class="mono">{{ path }}</span></span><span v-else>{{ t(runtime.platform === 'linux' ? '请通过 APP_IMPORT_PATHS 指定 YAML 导入目录；也可以直接上传配置文件。' : runtime.platform === 'docker' ? '请将 YAML 目录挂载到容器，并通过 APP_IMPORT_PATHS 指定；也可以直接上传配置文件。' : '如需从 NAS 共享目录导入 YAML，请到 fnOS「系统设置 → 应用 → Clash for fnos → 访问权限」添加文件夹。') }}</span></div><div class="local-processes"><div v-if="discovery.runtime" class="local-process" :class="{ managed: discovery.runtime.mode === 'managed' && discovery.runtime.running, none: !discovery.runtime.running }"><div class="local-process-dot" :class="{ off: !discovery.runtime.running }" /><div class="local-process-main"><strong>{{ t(runtimeTitle(discovery.runtime)) }}</strong><div class="mono muted local-process-args" :title="runtimeDetail(discovery.runtime)">{{ runtimeDetail(discovery.runtime) }}</div></div><span class="tag">{{ t(runtimeTag(discovery.runtime)) }}</span></div><template v-else><div v-for="process in discovery.processes || []" :key="process.pid" class="local-process"><div class="local-process-dot" /><div class="local-process-main"><strong>PID {{ t(process.pid) }} · {{ process.exe || 'mihomo' }}</strong><div class="mono muted local-process-args">{{ (process.args || []).join(' ') }}</div></div><span class="tag">{{ t(process.containerized ? '容器进程' : '主机进程') }}</span></div><div v-if="!discovery.processes?.length" class="local-process none"><div class="local-process-dot off" /><div><strong>{{ t("未获取到 Mihomo 运行状态") }}</strong><div class="muted">{{ t("仍会继续检查常见 config.yaml 路径") }}</div></div></div></template></div><div v-if="discovery.candidates?.length" class="local-config-list"><div v-for="candidate in discovery.candidates" :key="candidate.path" class="local-config-row"><div class="local-config-icon">Y</div><div class="local-config-main"><div class="local-config-path mono" :title="candidate.path">{{ candidate.path }}</div><div class="local-config-meta"><span>{{ t(candidate.source || '检测') }}</span><span v-if="candidate.namespace === 'process-root'">{{ t("进程根目录") }}</span><span v-if="candidate.size">{{ t(formatBytes(candidate.size)) }}</span><span v-if="candidate.mtime">{{ t(formatTime(candidate.mtime)) }}</span></div></div><div class="local-config-state" :class="candidate.readable ? 'good' : candidate.exists ? 'bad' : 'muted'"><span v-if="importedPaths.has(candidate.path)" class="local-imported">{{ t("已导入") }}</span> {{ t(candidateState(candidate)) }}</div><div class="actions local-config-actions"><template v-if="candidate.readable && candidate.token"><button class="ghost small" :disabled="Boolean(busyId)" @click="importNas(candidate, false)">{{ t("导入") }}</button><button class="success small" :disabled="Boolean(busyId)" @click="importNas(candidate, true)">{{ t("导入并应用") }}</button></template><button v-else class="ghost small" disabled>{{ t(candidateState(candidate)) }}</button></div></div></div></AsyncState></div>
<BaseModal :open="modal === 'remote'" :title="t(editing ? '编辑订阅' : '添加远程订阅')" @close="modal = null"><div class="hint" style="margin-bottom:14px">{{ t(editing ? '修改订阅信息后保存；订阅内容将在下次更新时重新下载。' : '填写远程订阅信息，添加后会立即尝试下载一次。') }}</div><div class="form-grid"><div class="field"><label>{{ t("名称") }}</label><input v-model="form.name" :placeholder="t('例如：机场订阅')"></div><div class="field"><label>{{ t("更新间隔（分钟）") }}</label><input v-model.number="form.intervalMinutes" type="number" min="5"></div><div class="field full"><label>{{ t("订阅 URL") }}</label><input v-model="form.url" placeholder="https://..."></div><div class="field full"><div class="hint">{{ t("自动更新顺序：直连 → 当前 Mihomo mixed-port → 系统 HTTP/HTTPS 代理。") }}</div></div><div class="field profile-checkbox-field"><label class="profile-checkbox-label"><input v-model="form.autoUpdate" type="checkbox"><span>{{ t("自动更新") }}</span></label></div><div class="field profile-checkbox-field"><label class="profile-checkbox-label"><input v-model="form.autoApply" type="checkbox"><span>{{ t("当前配置更新后自动应用") }}</span></label></div></div><div class="actions" style="margin-top:16px"><button class="small" :disabled="busyId === 'remote'" @click="saveRemote">{{ t(busyId === 'remote' ? '处理中…' : editing ? '保存修改' : '添加订阅') }}</button><button class="ghost small" @click="modal = null">{{ t("取消") }}</button></div></BaseModal>
  <BaseModal :open="modal === 'file'" :title="t('从当前电脑导入 YAML')" @close="modal = null"><div class="hint" style="margin-bottom:12px">{{ t("这里选择的是当前浏览器所在电脑上的文件；服务端本机配置请使用页面底部的自动扫描。") }}</div><div class="field"><label>{{ t("名称") }}</label><input v-model="fileName"></div><div class="field" style="margin-top:10px"><label>{{ t("选择文件") }}</label><input type="file" accept=".yaml,.yml,.txt" @change="selectedFile = ($event.target as HTMLInputElement).files?.[0] || null"></div><div class="actions" style="margin-top:16px"><button :disabled="busyId === 'file'" @click="importFile">{{ t(busyId === 'file' ? '处理中…' : '导入') }}</button><button class="ghost" @click="modal = null">{{ t("取消") }}</button></div></BaseModal>
  <ProfileExtensionEditor :open="Boolean(extensionKind && (extensionGlobal || extensionProfile))" :profile="extensionProfile" :kind="extensionKind" :global="extensionGlobal" @close="closeExtension" />
</template>
