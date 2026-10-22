<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, errorMessage } from '@/services/api'
import { t } from '@/services/i18n'
import { formatBytes } from '@/services/format'
import { observeDevices, type MonitorHost, type ObservedConnection } from '@/services/bypass-monitor'
import AppSelect from '@/components/AppSelect.vue'

const host = ref<MonitorHost | null>(null)
const runtimeTun = ref<boolean | null>(null)
const connections = ref<ObservedConnection[]>([])
const hostError = ref(''), runtimeError = ref(''), connectionError = ref('')
const refreshing = ref(false), updatedAt = ref(''), selected = ref('')
const controller = new AbortController()
let timer: ReturnType<typeof setTimeout> | undefined
let disposed = false
const devices = computed(() => observeDevices(connections.value, host.value?.interfaces || []))
const observed = computed(() => devices.value.some(device => device.tunCount > 0))
const selectedInterface = computed(() => host.value?.interfaces.find(i => `${i.name}/${i.address}` === selected.value))
const config = computed(() => host.value?.config)
function boolLabel(value: boolean | null | undefined): string { return value === true ? '已开启' : value === false ? '未开启' : '待验证' }
async function load() {
  if (refreshing.value || disposed) return
  refreshing.value = true
  const timeout = new AbortController()
  const abort = () => timeout.abort()
  controller.signal.addEventListener('abort', abort, { once: true })
  const deadline = setTimeout(abort, 8000)
  const results = await Promise.allSettled([
    api<MonitorHost>('/api/network/bypass', { signal: timeout.signal }),
    api<{ configs?: { tun?: { enable?: boolean } } }>('/api/status', { signal: timeout.signal }),
    api<{ connections?: ObservedConnection[] }>('/api/connections', { signal: timeout.signal }),
  ])
  clearTimeout(deadline)
  controller.signal.removeEventListener('abort', abort)
  if (disposed) return
  const [h, r, c] = results
  if (h.status === 'fulfilled') {
    host.value = h.value; hostError.value = ''
    if (!h.value.interfaces.some(i => `${i.name}/${i.address}` === selected.value)) {
      const first = h.value.interfaces[0]; selected.value = first ? `${first.name}/${first.address}` : ''
    }
  } else { host.value = null; hostError.value = errorMessage(h.reason) }
  if (r.status === 'fulfilled') { runtimeTun.value = typeof r.value.configs?.tun?.enable === 'boolean' ? r.value.configs.tun.enable : null; runtimeError.value = '' }
  else { runtimeTun.value = null; runtimeError.value = errorMessage(r.reason) }
  if (c.status === 'fulfilled') { connections.value = c.value.connections || []; connectionError.value = '' }
  else { connections.value = []; connectionError.value = errorMessage(c.reason) }
  updatedAt.value = new Date().toLocaleTimeString()
  refreshing.value = false
}
async function poll() { await load(); if (!disposed) timer = setTimeout(poll, 10000) }
onMounted(poll)
onBeforeUnmount(() => { disposed = true; clearTimeout(timer); controller.abort() })
defineExpose({ refreshPage: load })
</script>

<template>
  <div class="bypass-monitor">
    <div class="section-head"><div><h2>{{ t('旁路由检查与监控') }}</h2><p>{{ t('只读取现有配置和连接，每 10 秒刷新；不会修改网络设置。') }}</p></div><span class="tag">{{ t(refreshing ? '检查中…' : '只读检查') }}</span></div>
    <p v-if="host?.legacyJournal" class="warn-text" role="alert">{{ t('检测到旧开发版的旁路由恢复记录，请先使用旧版本停止并恢复；本页不会自动修改网络。') }}</p>
    <div class="monitor-summary" role="status"><strong>{{ t(connectionError || hostError ? '流量状态待验证' : observed ? '已观察到局域网 TUN 连接' : '尚未观察到局域网 TUN 连接') }}</strong><span>{{ t('仅代表当前可见连接，不证明全部设备、全部流量已接管。') }}</span><small v-if="updatedAt">{{ t('最近检查') }} {{ updatedAt }}</small></div>
    <div class="card monitor-checks">
      <h3>{{ t('接管条件') }}</h3>
      <dl class="check-list">
        <div><dt>{{ t('TUN 配置') }}</dt><dd>{{ t(boolLabel(config?.tun)) }}</dd></div>
        <div><dt>{{ t('TUN 运行态') }}</dt><dd>{{ t(boolLabel(runtimeTun)) }}<small v-if="runtimeError">{{ t('Controller 不可用，无法读取实际运行态。') }}</small></dd></div>
        <div><dt>Auto Route</dt><dd>{{ t(boolLabel(config?.autoRoute)) }}</dd></div>
        <div><dt>Auto Redirect</dt><dd>{{ t(boolLabel(config?.autoRedirect)) }}</dd></div>
        <div><dt>{{ t('DNS 配置') }}</dt><dd>{{ t(boolLabel(config?.dnsEnabled)) }}<small>{{ config?.dnsListen || t('未读到监听地址') }} · {{ t('设备可达性待验证') }}</small></dd></div>
        <div><dt>{{ t('DNS 劫持配置') }}</dt><dd>{{ config?.dnsHijack?.join(', ') || t('未读到配置') }}</dd></div>
        <div><dt>{{ t('TUN 排除网段') }}</dt><dd>{{ config?.routeExclude?.join(', ') || t('未读到配置') }}</dd></div>
        <div><dt>{{ t('系统 IPv4 转发') }}</dt><dd>{{ t(boolLabel(host?.forwarding)) }}<small>{{ t('此项单独不能判定 TUN 能否代理其他设备。') }}</small></dd></div>
        <div><dt>{{ t('防火墙与 NAT') }}</dt><dd>{{ t(host?.firewall.readable ? '已读取，转发效果待验证' : '无法读取，待验证') }}<small v-if="host?.firewall.readable">{{ t('转发链') }} {{ host.firewall.forwardChains }} · {{ t('源 NAT 规则') }} {{ host.firewall.natRules }} · {{ t('丢弃规则') }} {{ host.firewall.dropRules }}</small></dd></div>
      </dl>
      <p v-if="hostError" class="warn-text" role="alert">{{ t(hostError) }}</p>
      <p v-else-if="host?.configError" class="muted">{{ t('启动配置不可读取或格式无效，配置项显示为待验证。') }}</p>
      <p class="muted">{{ t('未显式配置的选项显示为待验证。规则数量不代表放行或拦截结果，也不表示必须额外添加 NAT。') }}</p>
      <a href="#settings">{{ t('前往设置检查 TUN 与 DNS') }}</a>
    </div>
    <div class="card monitor-devices">
      <h3>{{ t('局域网来源的当前连接') }}</h3><p class="muted">{{ t('按源 IP 汇总当前连接；TUN、HTTP、SOCKS 等入站分别展示，显式代理连接不作为 TUN 接管证据。') }}</p>
      <p v-if="connectionError" class="warn-text" role="alert">{{ t('连接数据不可用，当前接管状态待验证。') }}</p>
      <div v-else-if="devices.length" class="monitor-table"><table><thead><tr><th>{{ t('来源 IP') }}</th><th>{{ t('当前连接') }}</th><th>{{ t('入站类型') }}</th><th>{{ t('命中规则') }}</th><th>{{ t('出口链') }}</th><th>{{ t('当前连接累计流量') }}</th></tr></thead><tbody><tr v-for="device in devices" :key="device.ip"><td>{{ device.ip }}</td><td>{{ device.count }}<small>TUN {{ device.tunCount }}</small></td><td>{{ device.types.join(', ') || t('未知') }}</td><td>{{ device.rules.join(' / ') || '—' }}</td><td>{{ device.chains.join(' / ') || '—' }}</td><td>↑ {{ formatBytes(device.upload) }}<small>↓ {{ formatBytes(device.download) }}</small></td></tr></tbody></table></div>
      <p v-else class="muted">{{ t('当前未看到局域网其他设备的连接。请让设备发起访问后刷新；没有连接不代表不支持旁路由。') }}</p>
      <a href="#connections">{{ t('查看全部连接') }}</a>
    </div>
    <div class="card monitor-guide"><h3>{{ t('设备接入与验证') }}</h3>
      <div v-if="host?.interfaces.length" class="guide-interface"><span>{{ t('接入网卡') }}</span><AppSelect v-model="selected" :label="t('接入网卡')" :options="host.interfaces.map(i => ({ value: `${i.name}/${i.address}`, label: `${i.name} · ${i.address}` }))" /></div>
      <p>{{ t('设备默认网关可填写所选 NAS IPv4；DNS 仅在 NAS 已提供可达的 DNS 服务时指向该地址。') }}</p>
      <dl class="guide-address"><div><dt>{{ t('NAS IPv4') }}</dt><dd>{{ selectedInterface?.address || '—' }}</dd></div><div><dt>{{ t('局域网网段') }}</dt><dd>{{ selectedInterface?.cidr || '—' }}</dd></div></dl>
      <p>{{ t('在设备上访问网站，然后观察此页是否出现该设备源 IP 的 TUN 连接、命中规则和出口链。') }}</p>
      <p class="muted">{{ t('IPv6、同网段直连和被排除的目标可能绕过 TUN；本页不证明这些流量已接管。') }}</p>
    </div>
  </div>
</template>
<style scoped>
.bypass-monitor { display: grid; gap: 14px; }
.monitor-checks, .monitor-devices, .monitor-guide { padding: 18px; min-width: 0; }
.monitor-summary { display:flex; flex-wrap:wrap; gap:8px 20px; align-items:baseline; }
.monitor-summary span, .monitor-summary small { color:var(--muted); }
.check-list { margin:12px 0; }
.check-list > div { display:grid;grid-template-columns:minmax(140px,1fr) 2fr;gap:12px;padding:10px 0;border-bottom:1px solid var(--line); }
.check-list dd, .guide-address dd { margin:0;overflow-wrap:anywhere; }
small { display:block;color:var(--muted);margin-top:5px; }
.monitor-table { overflow-x:auto; }
.monitor-table td { max-width:230px;overflow-wrap:anywhere;vertical-align:top; }
.monitor-table th { white-space:nowrap; }
.guide-interface { display:grid;grid-template-columns:140px minmax(0,340px);align-items:center;gap:12px;margin:14px 0; }
.guide-address { display:flex;gap:30px;flex-wrap:wrap; }
.guide-address dd { margin-top:6px; }
p { line-height:1.65; }
@media(max-width:600px) { .check-list > div,.guide-interface { grid-template-columns:1fr;gap:6px; } }
</style>
