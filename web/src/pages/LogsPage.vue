<script setup lang="ts">
import { t } from '@/services/i18n'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import HighlightText from '@/components/HighlightText.vue'
import SequenceFormDialog from '@/components/SequenceFormDialog.vue'
import { api, APP_PREFIX, errorMessage, isAbortError } from '@/services/api'
import { containsLog, displayLogTime, logDateBoundary, normalizeLog, plainCoreOutput, type CoreLogLine, type NormalizedLog } from '@/services/logs'
import { notify } from '@/services/toast'
import type { LogItem } from '@/types/api'

type Policy = { days: number; maxMiB: number }
type Settings = { history: Policy; core: Policy; saveLevel: string }
type Stats = { size: number; maxBytes: number; oldest?: string; available?: boolean; error?: string }
type Status = { settings: Settings; history: Stats; core: Stats }
type HistoryPage = Stats & { items?: LogItem[]; nextCursor: string; hasMore: boolean }
type CorePage = Stats & { lines: CoreLogLine[]; nextCursor: string; hasMore: boolean }
const source = ref<'runtime' | 'core'>('runtime'), coreLines = ref<CoreLogLine[]>([])
const isCore = computed(() => source.value === 'core')
const items = ref<NormalizedLog[]>([]), query = ref(''), limit = ref(800), level = ref('info'), running = ref(true), wrapLines = ref(false), loading = ref(true), error = ref('')
const queryDirty = ref(false)
const historyMode = ref(false), from = ref(''), to = ref(''), cursors = ref<string[]>(['']), pageIndex = ref(0), nextCursor = ref('')
const settingsOpen = ref(false), status = ref<Status | null>(null), draft = ref<Settings | null>(null), settingsBusy = ref(false), settingsError = ref('')
const box = ref<HTMLElement | null>(null)
const filtered = computed(() => historyMode.value ? items.value : items.value.filter(item => containsLog(item, query.value)))
const visible = computed(() => historyMode.value ? filtered.value : filtered.value.slice(-limit.value))
const visibleCore = computed(() => historyMode.value ? coreLines.value : coreLines.value.filter(line => line.message.toLowerCase().includes(query.value.trim().toLowerCase())))
const currentStats = computed(() => isCore.value ? status.value?.core : status.value?.history)
const sourceDescription = computed(() => t(!isCore.value ? 'Mihomo Controller 运行日志' : historyMode.value ? '查询托管 Core 保留的原始输出' : running.value ? '托管 Core 原始输出 · 每 3 秒刷新' : '托管 Core 原始输出 · 已暂停刷新'))
const summary = computed(() => {
  if (loading.value) return t('正在读取日志…')
  if (error.value) return t(`读取失败：${error.value}`)
  if (queryDirty.value) return t('筛选已更改，请点击查询')
  if (isCore.value) return t(`本页 ${visibleCore.value.length} 行 Core 输出`)
  return t(historyMode.value ? `本页 ${visible.value.length} 条历史日志` : `显示 ${visible.value.length} / ${filtered.value.length} 条 · 最近 ${items.value.length} 条日志中筛选`)
})
let stream: EventSource | null = null, controller: AbortController | null = null, settingsController: AbortController | null = null, renderFrame = 0, statusTimer = 0, coreTimer = 0
const bytes = (value: number) => value < 1024 ? `${value} B` : value < 1048576 ? `${(value / 1024).toFixed(1)} KiB` : `${(value / 1048576).toFixed(2)} MiB`
function scrollBottom() { cancelAnimationFrame(renderFrame); renderFrame = requestAnimationFrame(() => nextTick(() => { if (box.value) box.value.scrollTop = box.value.scrollHeight })) }
function stopStream() { stream?.close(); stream = null; window.clearTimeout(coreTimer) }
function startStream() {
  stopStream()
  if (!running.value || historyMode.value) return
  if (isCore.value) { coreTimer = window.setTimeout(() => void load(false, true), 3000); return }
  stream = new EventSource(`${APP_PREFIX}/api/stream/logs?level=${encodeURIComponent(level.value)}`)
  stream.onmessage = event => {
    try { items.value.push(normalizeLog(JSON.parse(event.data) as LogItem)); if (items.value.length > 2000) items.value.shift(); scrollBottom() }
    catch { /* Keep streaming after malformed events. */ }
  }
}
async function load(reset = true, background = false) {
  queryDirty.value = false; stopStream(); controller?.abort(); const request = new AbortController(); controller = request; if (!background) loading.value = true
  if (reset) { cursors.value = ['']; pageIndex.value = 0 }
  try {
    if (!isCore.value && historyMode.value && from.value && to.value && from.value > to.value) throw new Error(t('开始日期不能晚于结束日期'))
    const params = new URLSearchParams({ limit: String(isCore.value || historyMode.value ? limit.value : 2000) })
    if (!isCore.value) params.set('level', level.value)
    if (historyMode.value) {
      params.set('search', query.value); params.set('cursor', cursors.value[pageIndex.value] || '')
      if (!isCore.value && from.value) params.set('from', logDateBoundary(from.value))
      if (!isCore.value && to.value) params.set('to', logDateBoundary(to.value, true))
    }
    if (isCore.value) {
      const data = await api<CorePage>(`/api/logs/core?${params}`, { signal: request.signal })
      if (controller !== request || request.signal.aborted) return
      coreLines.value = data.lines.map(line => ({ ...line, message: plainCoreOutput(line.message) })); nextCursor.value = data.hasMore ? data.nextCursor : ''; error.value = ''
      if (status.value) status.value.core = { ...data, available: true }
    } else {
      const data = await api<HistoryPage>(`/api/logs/history?${params}`, { signal: request.signal })
      if (controller !== request || request.signal.aborted) return
      items.value = (data.items || []).map(normalizeLog); nextCursor.value = data.hasMore ? data.nextCursor : ''; error.value = ''
      if (status.value) status.value.history = data
    }
    startStream(); scrollBottom()
  } catch (cause) { if (controller === request && !isAbortError(cause)) { items.value = []; coreLines.value = []; nextCursor.value = ''; error.value = errorMessage(cause); if (!background) notify(error.value, true); startStream() } }
  finally { if (controller === request) loading.value = false }
}
function changeMode() { historyMode.value = !historyMode.value; void load() }
function changeSource(value: 'runtime' | 'core') { if (source.value === value) return; stopStream(); controller?.abort(); source.value = value; items.value = []; coreLines.value = []; error.value = ''; void load() }
function older() { if (!nextCursor.value) return; cursors.value = [...cursors.value.slice(0, pageIndex.value + 1), nextCursor.value]; pageIndex.value++; void load(false) }
function newer() { if (pageIndex.value > 0) { pageIndex.value--; void load(false) } }
async function clearHistory() {
  try { await api(isCore.value ? '/api/logs/core' : '/api/logs/history', { method: 'DELETE' }); notify(isCore.value ? 'Core 输出日志已清空' : '历史日志已清空'); await load(); await readSettings(false) }
  catch (cause) { notify(errorMessage(cause), true) }
}
function toggle() { running.value = !running.value; if (!running.value) { stopStream(); if (isCore.value) { controller?.abort(); loading.value = false } } else if (isCore.value) void load(); else startStream() }
async function readSettings(edit = true) {
  settingsController?.abort(); const request = new AbortController(); settingsController = request; settingsBusy.value = true; settingsError.value = ''
  if (edit) draft.value = null
  try { status.value = await api<Status>('/api/logs/settings', { signal: request.signal }); if (edit) draft.value = JSON.parse(JSON.stringify(status.value.settings)) }
  catch (cause) { if (!isAbortError(cause)) settingsError.value = errorMessage(cause) }
  finally { if (settingsController === request) settingsBusy.value = false }
}
function openSettings() { settingsOpen.value = true; void readSettings() }
defineExpose({ refreshPage: async () => { await load(); await readSettings(false) } })
function closeSettings() { if (!settingsBusy.value) settingsOpen.value = false }
async function saveSettings() {
  if (!draft.value) return
  settingsBusy.value = true; settingsError.value = ''; settingsController?.abort(); settingsController = new AbortController()
  try { status.value = await api<Status>('/api/logs/settings', { method: 'PUT', body: JSON.stringify(draft.value), signal: settingsController.signal }); settingsOpen.value = false; notify('日志设置已保存'); await load() }
  catch (cause) { if (!isAbortError(cause)) settingsError.value = errorMessage(cause) }
  finally { settingsBusy.value = false }
}
async function clearCore() {
  settingsBusy.value = true; settingsError.value = ''; settingsController?.abort(); settingsController = new AbortController()
  try { const core = await api<Stats>('/api/logs/core', { method: 'DELETE', signal: settingsController.signal }); if (status.value) status.value.core = core; notify('Core 输出日志已清空'); if (isCore.value) await load() }
  catch (cause) { if (!isAbortError(cause)) settingsError.value = errorMessage(cause) }
  finally { settingsBusy.value = false }
}
watch([query, from, to], () => { if (historyMode.value) { queryDirty.value = true; error.value = ''; controller?.abort(); loading.value = false; items.value = []; coreLines.value = []; nextCursor.value = ''; cursors.value = ['']; pageIndex.value = 0 } })
onMounted(() => { void load(); void readSettings(false); statusTimer = window.setInterval(() => { if (!settingsOpen.value && !settingsBusy.value) void readSettings(false) }, 30000) })
onBeforeUnmount(() => { controller?.abort(); settingsController?.abort(); stopStream(); cancelAnimationFrame(renderFrame); window.clearInterval(statusTimer) })
</script>

<template>
  <Teleport defer to="#page-actions"><div class="log-tools">
    <input v-model="query" type="search" class="log-search" :placeholder="t(historyMode ? '搜索保留的历史日志' : '搜索最近日志')" :aria-label="t('搜索日志')" @keydown.enter="historyMode && load()">
    <select v-model.number="limit" class="log-limit-select" :aria-label="t('显示行数')" @change="(isCore || historyMode) && load()"><option v-for="value in [100, 200, 500, 800, 2000]" :key="value" :value="value">{{ value }} {{ t('行') }}</option></select>
    <select v-if="!isCore" v-model="level" class="log-level-select" :aria-label="t('显示级别')" @change="load()"><option v-for="value in ['debug', 'info', 'warning', 'error']" :key="value">{{ value }}</option></select>
    <button class="ghost" @click="changeMode">{{ t(historyMode ? '实时日志' : '历史查询') }}</button><button class="ghost" @click="openSettings">{{ t('日志设置') }}</button>
    <button class="ghost" @click="clearHistory">{{ t(isCore ? '清空 Core 输出日志' : '清空历史日志') }}</button><button v-if="!historyMode" @click="toggle">{{ t(running ? '停止' : '继续') }}</button>
  </div></Teleport>
  <form v-if="historyMode" class="log-history-tools" @submit.prevent="load()">
    <div class="log-history-query">
      <template v-if="!isCore"><label>{{ t('开始日期') }}<input v-model="from" type="date"></label><label>{{ t('结束日期') }}<input v-model="to" type="date"></label></template>
      <button type="submit" :disabled="loading">{{ t('查询') }}</button>
    </div>
    <div class="log-history-pagination">
      <button type="button" class="ghost" :disabled="loading || pageIndex === 0" @click="newer">{{ t('较新一页') }}</button>
      <button type="button" class="ghost" :disabled="loading || !nextCursor" @click="older">{{ t('更早一页') }}</button>
      <span class="muted">{{ t(`第 ${pageIndex + 1} 页`) }}</span>
    </div>
  </form>
  <div class="log-summary">
    <div class="log-summary-main">
      <div class="log-source-switch" role="group" :aria-label="t('日志来源')" :title="sourceDescription">
        <button type="button" :class="{ active: !isCore }" :aria-pressed="!isCore" @click="changeSource('runtime')">{{ t('运行日志') }}</button>
        <button type="button" :class="{ active: isCore }" :aria-pressed="isCore" @click="changeSource('core')">{{ t('Core 输出') }}</button>
      </div>
      <span class="muted log-status-text" :title="summary + (currentStats ? ` · ${bytes(currentStats.size)} / ${bytes(currentStats.maxBytes)}` : '')">{{ summary }}<template v-if="currentStats"> · {{ bytes(currentStats.size) }} / {{ bytes(currentStats.maxBytes) }}</template></span>
    </div>
    <label class="log-wrap-control"><span>{{ t('自动换行') }}</span><span class="switch quick-switch"><input v-model="wrapLines" type="checkbox"><span /></span></label>
  </div>
  <p v-if="isCore && currentStats?.error" class="error log-storage-error" role="alert">{{ currentStats.error }}</p>
  <div ref="box" class="logs logs-full persistent-horizontal-scrollbar" :class="{ 'wrap-lines': wrapLines, 'core-output': isCore }"><template v-if="isCore"><div v-for="line in visibleCore" :key="line.key" class="core-log-line"><HighlightText :text="line.message" :query="query" /><span v-if="line.truncated" class="muted"> {{ t('（此行过长，已截断）') }}</span></div></template><template v-else><div v-for="(item, index) in visible" :key="`${index}-${item.time}`" class="log-line" :class="`log-${item.level}`"><span :title="item.time"><HighlightText :text="item.time" :query="query" /></span><span><HighlightText :text="item.level" :query="query" /></span><span><HighlightText :text="item.message" :query="query" /></span></div></template><div v-if="!(isCore ? visibleCore.length : visible.length) && !loading" class="empty">{{ t(error ? '日志读取失败，请重新读取' : queryDirty ? '筛选已更改，请点击查询' : query ? '没有匹配的日志' : isCore ? '暂无托管 Core 输出日志' : '暂无日志') }}</div></div>
  <SequenceFormDialog :open="settingsOpen" :title="t('日志设置')" compact @close="closeSettings">
    <p class="muted log-policy-help">{{ t('超过保留天数或容量上限时，自动清理最旧日志。保留天数为 0 时不限时间，仍受容量限制。') }}</p>
    <p v-if="settingsError" class="error" role="alert">{{ t(settingsError) }}</p><p v-if="settingsBusy && !draft" class="muted">{{ t('正在读取…') }}</p>
    <form v-if="draft && status" id="log-settings-form" class="log-settings-form" @submit.prevent="saveSettings">
      <fieldset :disabled="settingsBusy"><legend>{{ t('页面历史日志') }}</legend>
        <div class="log-policy-fields"><label>{{ t('保留天数') }}<input v-model.number="draft.history.days" type="number" min="0" max="365" step="1" required></label><label>{{ t('容量上限（MiB）') }}<input v-model.number="draft.history.maxMiB" type="number" min="1" max="1024" step="1" required></label><label>{{ t('保存级别') }}<select v-model="draft.saveLevel"><option v-for="value in ['info', 'warning', 'error', 'debug']" :key="value">{{ value }}</option></select></label></div>
        <p class="muted">{{ t('保存级别仅影响历史记录；实时显示级别可独立选择。Debug 适合临时排查。') }}</p>
        <p class="muted">{{ t('已使用：') }}{{ bytes(status.history.size) }} · {{ t('最早记录：') }}{{ status.history.oldest ? displayLogTime(status.history.oldest) : t('暂无日志') }}</p>
        <p v-if="status.history.error" class="error" role="alert">{{ t('历史日志保存失败：') }}{{ status.history.error }}</p>
      </fieldset>
      <fieldset :disabled="settingsBusy"><legend>{{ t('托管 Core 输出日志') }}</legend>
        <div class="log-policy-fields"><label>{{ t('保留天数') }}<input v-model.number="draft.core.days" type="number" min="0" max="365" step="1" required></label><label>{{ t('容量上限（MiB）') }}<input v-model.number="draft.core.maxMiB" type="number" min="1" max="1024" step="1" required></label></div>
        <p class="muted">{{ t('管理本应用托管 Core 的 stdout / stderr，不包含外部 Core 或容器运行时日志。') }}</p>
        <p v-if="status.core.error" class="error" role="alert">{{ t(status.core.error) }}</p>
        <div class="log-core-storage">
          <p v-if="status.core.available" class="muted">{{ t('已使用：') }}{{ bytes(status.core.size) }} · {{ t('最早文件更新时间：') }}{{ status.core.oldest ? displayLogTime(status.core.oldest) : t('暂无日志') }}</p>
          <button type="button" class="ghost danger" :disabled="!status.core.available" @click="clearCore">{{ t('清空 Core 输出日志') }}</button>
        </div>
      </fieldset>
    </form><button v-else-if="!settingsBusy" class="ghost" @click="readSettings()">{{ t('重新读取') }}</button>
    <template #footer><div class="log-settings-actions"><button type="button" class="ghost" :disabled="settingsBusy" @click="closeSettings">{{ t('取消') }}</button><button type="submit" form="log-settings-form" :disabled="settingsBusy || !draft">{{ t('保存设置') }}</button></div></template>
  </SequenceFormDialog>
</template>

<style scoped>
.log-summary-main{display:flex;align-items:center;gap:12px;flex:1;min-width:0}
.log-status-text{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.log-source-switch{display:flex;gap:2px;flex:none;height:28px;padding:2px;border:1px solid var(--line);border-radius:8px;background:var(--control-bg)}
.log-source-switch button{height:22px;min-height:22px;padding:0 8px;border:0;border-radius:5px;background:transparent;color:var(--muted);font-size:11px;line-height:22px;white-space:nowrap}
.log-source-switch button:hover{color:var(--text);background:rgba(124,156,255,.08)}
.log-source-switch button.active{color:var(--accent);background:rgba(124,156,255,.14)}
.core-log-line{white-space:pre;min-height:22px;padding:2px 0}.wrap-lines .core-log-line{white-space:pre-wrap;overflow-wrap:anywhere}.log-storage-error{font-size:12px;margin:6px 28px}
.log-history-tools{display:flex;flex-wrap:wrap;align-items:center;gap:8px 16px;min-height:42px;padding:6px 28px;border-bottom:1px solid var(--line)}
.log-history-query{display:flex;flex-wrap:wrap;align-items:center;gap:8px 12px;min-width:0}
.log-history-tools label{display:flex;align-items:center;gap:6px;font-size:11px;white-space:nowrap}
.log-history-tools input{width:132px;height:28px;min-height:28px;padding:0 8px;border-radius:7px;font-size:11px}
.log-history-tools button{height:28px;min-height:28px;padding:0 10px;border-radius:7px;font-size:11px;line-height:26px;white-space:nowrap}
.log-history-pagination{display:flex;align-items:center;gap:6px;margin-left:auto;font-size:11px;white-space:nowrap}
.log-history-pagination>span{margin-left:4px}
.log-policy-help{font-size:11px;line-height:1.5;margin:0 0 10px}
.log-settings-form fieldset{border:1px solid var(--line);border-radius:8px;margin:0 0 10px;padding:10px}
.log-settings-form fieldset:last-child{margin-bottom:0}
.log-settings-form legend{font-weight:600;font-size:13px;padding:0 4px}
.log-settings-form p{font-size:11px;line-height:1.5;margin:6px 0;overflow-wrap:anywhere}
.log-policy-fields{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}
.log-policy-fields label{display:flex;flex-direction:column;gap:4px;font-size:12px}
.log-policy-fields input,.log-policy-fields select{width:100%;min-width:0;height:28px;padding:0 8px;border-radius:6px;font-size:12px}
.log-core-storage{display:flex;flex-wrap:wrap;align-items:center;gap:6px 10px;margin-top:8px}
.log-core-storage p{flex:1;min-width:200px;margin:0}
.log-core-storage button,.log-settings-actions button{flex:none;height:28px;min-height:28px;padding:0 10px;border-radius:7px;font-size:11px;line-height:26px}
.log-settings-actions{width:100%;display:flex;justify-content:flex-end;gap:8px}
.error{color:var(--bad);overflow-wrap:anywhere}
@media(max-width:680px){.log-history-tools{padding:6px 16px}}
@media(max-width:540px){.log-policy-fields{grid-template-columns:1fr 1fr}}
</style>
