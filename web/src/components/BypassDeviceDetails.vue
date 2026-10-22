<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { api, errorMessage, isAbortError } from '@/services/api'
import { t } from '@/services/i18n'
import { formatBytes } from '@/services/format'
import { connectionExit, logFromDevice, type InventoryDevice, type ObservedConnection } from '@/services/bypass-monitor'
import { normalizeLog, type NormalizedLog } from '@/services/logs'
import type { LogItem } from '@/types/api'
import { notify } from '@/services/toast'

const props = defineProps<{ ip: string; device: InventoryDevice; connections: ObservedConnection[]; trafficAvailable: boolean }>()
const current = computed(() => props.connections.filter(c => c.metadata?.sourceIP === props.ip))
const rows = ref<NormalizedLog[]>([]), error = ref(''), loading = ref(false), running = ref(true)
const globalLogs = ref(false), errorsOnly = ref(false), readAt = ref('')
const visible = computed(() => rows.value.filter(row => !errorsOnly.value || /^(warn|error|fatal)$/.test(row.level)))
let request: AbortController | undefined, timer: ReturnType<typeof setTimeout> | undefined, disposed = false
async function load() {
  clearTimeout(timer); request?.abort()
  const active = new AbortController(); request = active; loading.value = true
  const deadline = setTimeout(() => active.abort(), 8000)
  try {
    const params = new URLSearchParams({ limit: '200', level: 'debug' })
    if (!globalLogs.value) params.set('search', props.ip)
    const data = await api<{ items?: LogItem[] }>(`/api/logs/history?${params}`, { signal: active.signal })
    if (disposed || request !== active) return
    rows.value = (data.items || []).map(normalizeLog).filter(row => globalLogs.value || logFromDevice(row.message, props.ip))
    error.value = ''; readAt.value = new Date().toLocaleTimeString()
  } catch (cause) {
    if (!disposed && request === active) { rows.value = []; error.value = isAbortError(cause) ? t('日志读取超时，请重试') : errorMessage(cause) }
  } finally {
    clearTimeout(deadline)
    if (!disposed && request === active) { loading.value = false; if (running.value) timer = setTimeout(load, 10000) }
  }
}
function toggle() { running.value = !running.value; if (running.value) void load(); else { clearTimeout(timer); request?.abort(); request = undefined; loading.value = false } }
function exportLogs() {
  const text = [t(globalLogs.value ? '全局日志，未归属到所选设备' : '设备源 IP 日志'), props.ip, ...visible.value.map(row => `${row.time} [${row.level}] ${row.message}`)].join('\n')
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' }))
  const link = document.createElement('a'); link.href = url; link.download = `bypass-${props.ip}.log`; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
}
async function copyLogs() {
  try { await navigator.clipboard.writeText(visible.value.map(row => `${row.time} [${row.level}] ${row.message}`).join('\n')); notify('日志已复制') }
  catch { notify('复制失败，请导出日志', true) }
}
watch(globalLogs, () => { rows.value = []; error.value = ''; readAt.value = ''; void load() }, { immediate: true })
onBeforeUnmount(() => { disposed = true; clearTimeout(timer); request?.abort() })
</script>

<template>
  <section class="device-details" :aria-label="t('设备诊断')">
    <dl class="device-facts">
      <div><dt>MAC</dt><dd>{{ device.mac || t('MAC 未知') }}</dd></div>
      <div><dt>{{ t('接入网卡') }}</dt><dd>{{ device.interface || '—' }}</dd></div>
      <div><dt>{{ t('最近观察') }}</dt><dd>{{ new Date(device.lastObserved).toLocaleString() }}</dd></div>
    </dl>
    <div class="connection-heading"><h3>{{ t('当前连接') }}</h3><span class="muted">{{ trafficAvailable ? current.length : '—' }}<template v-if="trafficAvailable"> · TUN {{ device.tunCount }}</template></span></div>
    <p v-if="!trafficAvailable" class="warn-text" role="alert">{{ t('连接数据不可用，当前接管状态待验证。') }}</p>
    <div v-else-if="current.length" class="detail-table"><table><thead><tr><th>{{ t('目标 / 协议') }}</th><th>{{ t('入站') }}</th><th>{{ t('命中规则') }}</th><th>{{ t('最终出口') }}</th><th>{{ t('累计流量') }}</th></tr></thead><tbody><tr v-for="(connection, index) in current" :key="connection.id || index"><td>{{ connection.metadata?.host || connection.metadata?.destinationIP || '—' }}<small>{{ connection.metadata?.destinationPort || '—' }} · {{ connection.metadata?.network || '—' }}</small></td><td>{{ connection.metadata?.type || t('待确认') }}</td><td>{{ [connection.rule, connection.rulePayload].filter(Boolean).join(': ') || '—' }}</td><td><span class="tag">{{ t(connectionExit(connection)) }}</span><small v-if="connection.chains?.length && !/^(DIRECT|REJECT(?:-DROP)?)$/i.test(connection.chains.join(''))">{{ connection.chains.join(' ← ') }}</small></td><td>↑ {{ formatBytes(connection.upload || 0) }}<small>↓ {{ formatBytes(connection.download || 0) }}</small></td></tr></tbody></table></div>
    <p v-else class="muted">{{ t('当前没有该设备的可见连接。请在设备上发起访问后刷新；没有记录不能直接判定流量未经过 NAS。') }}</p>
    <div class="log-heading"><h3>{{ t('相关日志') }}</h3><span class="muted">{{ readAt || '—' }}</span></div>
    <div class="diagnostic-tools"><button type="button" class="ghost" :disabled="loading" @click="load">{{ t('刷新日志') }}</button><button type="button" class="ghost" @click="toggle">{{ t(running ? '暂停刷新' : '继续刷新') }}</button><label><input v-model="errorsOnly" type="checkbox">{{ t('仅警告与错误') }}</label><label><input v-model="globalLogs" type="checkbox">{{ t('查看全局日志') }}</label><button type="button" class="ghost" :disabled="!visible.length" @click="copyLogs">{{ t('复制日志') }}</button><button type="button" class="ghost" :disabled="!visible.length" @click="exportLogs">{{ t('导出日志') }}</button></div>
    <p v-if="globalLogs" class="warn-text">{{ t('全局日志包含其他设备及系统消息，不代表所选设备发生了错误。') }}</p>
    <p v-if="error" class="warn-text" role="alert">{{ error }}</p>
    <div class="diagnostic-logs" role="region" :aria-label="t('相关日志')" tabindex="0"><div v-for="(row, index) in visible" :key="index" class="diagnostic-line"><time>{{ row.time }}</time><span :class="{ 'warn-text': /^(warn|error|fatal)$/.test(row.level) }">{{ row.level }}</span><span>{{ row.message }}</span></div><p v-if="!visible.length" class="muted">{{ t(loading ? '正在读取日志…' : error ? '日志读取失败，请重新读取' : '当前范围内没有匹配日志，不代表设备没有故障。') }}</p></div>
    <details class="diagnostic-help"><summary>{{ t('诊断说明') }}</summary>
      <p>{{ t('已保存的 Controller 日志') }} · {{ t('每 10 秒读取最近最多 200 条保留日志；设备筛选仅匹配可识别的源 IP，无法归属的消息请查看全局日志。不会开启 Debug 或修改保存级别。') }}</p>
      <p>{{ t('当前连接显示入站、规则与出口；命中代理出口不代表请求已成功。') }}</p>
      <p>{{ t('流量若在系统转发或防火墙处被拦截，Mihomo 可能没有失败日志；请结合基础检查，必要时抓包验证。') }} <a href="#logs">{{ t('查看完整日志') }}</a></p>
    </details>
  </section>
</template>
<style scoped>
.device-details{min-width:0}.device-facts{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px 20px;margin:0 0 16px}.device-facts dt{font-size:11px;color:var(--muted);margin-bottom:4px}.device-facts dd{margin:0;font-size:12px;overflow-wrap:anywhere}.connection-heading{display:flex;align-items:center;gap:10px;margin-bottom:10px;font-size:12px}.connection-heading h3{margin:0;font-size:13px}.detail-table{overflow:auto;max-height:320px}.detail-table td{max-width:280px;overflow-wrap:anywhere;vertical-align:top}small{display:block;color:var(--muted);margin-top:5px}.log-heading{display:flex;gap:12px;align-items:center;flex-wrap:wrap;margin-top:18px}.log-heading{font-size:12px}.log-heading h3{margin:0;font-size:13px}.diagnostic-tools{display:flex;gap:8px 14px;align-items:center;flex-wrap:wrap;margin:12px 0}.diagnostic-tools label{display:flex;gap:6px;align-items:center;font-size:12px}.diagnostic-tools input{width:14px;height:14px;min-height:0;margin:0}.diagnostic-tools button{font-size:12px;min-height:30px;padding:4px 10px}.diagnostic-logs{min-height:220px;max-height:360px;overflow:auto;padding:12px;border:1px solid var(--line);border-radius:8px;background:var(--control-bg);font:12px/1.6 ui-monospace,monospace}.diagnostic-line{display:grid;grid-template-columns:150px 45px minmax(220px,1fr);gap:10px;padding:3px 0}.diagnostic-line>span:last-child{white-space:pre-wrap;overflow-wrap:anywhere}.diagnostic-line time{color:var(--muted)}.diagnostic-help{margin-top:12px;color:var(--muted);font-size:12px;line-height:1.6}.diagnostic-help summary{cursor:pointer}.diagnostic-help p{margin:8px 0 0}@media(max-width:650px){.diagnostic-logs{min-height:160px;max-height:300px}.device-facts{grid-template-columns:1fr}.diagnostic-line{grid-template-columns:1fr}.diagnostic-line time,.diagnostic-line>span{display:block}.section-head{flex-wrap:wrap}}
</style>
