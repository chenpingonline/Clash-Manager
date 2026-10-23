<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, errorMessage } from '@/services/api'
import { t } from '@/services/i18n'
import { formatBytes } from '@/services/format'
import { bypassInventory, isContainerInterface, type InventoryDevice, type MonitorHost, type ObservedConnection } from '@/services/bypass-monitor'
import BypassDeviceDetails from '@/components/BypassDeviceDetails.vue'
import SequenceFormDialog from '@/components/SequenceFormDialog.vue'
import AppSelect from '@/components/AppSelect.vue'
import HelpPopover from '@/components/HelpPopover.vue'

const host = ref<MonitorHost | null>(null)
const runtimeTun = ref<boolean | null>(null)
const connections = ref<ObservedConnection[]>([])
const hostError = ref(''), runtimeError = ref(''), connectionError = ref('')
const refreshing = ref(false), updatedAt = ref(''), selected = ref('')
const controller = new AbortController()
let timer: ReturnType<typeof setTimeout> | undefined
let disposed = false
const devices = ref<InventoryDevice[]>([])
const selectedDevice = ref('')
const showContainers = ref(false)
const onlyConnected = ref(true)
const detailTabs = [{ key: 'checks', label: '基础检查' }, { key: 'guide', label: '设备接入' }] as const
const detailTab = ref<(typeof detailTabs)[number]['key']>('checks')
function navigateDetailTab(event: KeyboardEvent, index: number) {
  let next = index
  if (event.key === 'ArrowRight' || event.key === 'ArrowLeft') next = (index + 1) % detailTabs.length
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = detailTabs.length - 1
  else return
  event.preventDefault()
  detailTab.value = detailTabs[next]!.key
  document.getElementById(`bypass-tab-${detailTab.value}`)?.focus()
}
const lanDevices = computed(() => devices.value.filter(device => !isContainerInterface(device.interface)))
const containerCount = computed(() => devices.value.length - lanDevices.value.length)
const visibleDevices = computed(() => (showContainers.value ? devices.value : lanDevices.value)
  .filter(device => !onlyConnected.value || (!connectionError.value && device.recentTraffic)))
const activeDevice = computed(() => devices.value.find(device => device.ip === selectedDevice.value))
const lanInterfaces = computed(() => (host.value?.interfaces || []).filter(i => !isContainerInterface(i.name)))
const diagnosticHints = computed(() => {
  const hints: string[] = []
  if (runtimeTun.value === false) hints.push('TUN 当前未运行，网关流量的透明接管待验证。')
  if (host.value?.forwarding === false) hints.push('IPv4 转发已关闭，可能影响设备的 UDP 与 DNS；TCP 可用也不代表旁路接入完整。')
  if (config.value?.autoRoute === false) hints.push('自动路由未开启，需要核对是否存在其他流量接管规则。')
  if (config.value?.autoRedirect === false) hints.push('Auto Redirect 未开启，发往 NAS 的 DNS 和部分路由路径需要另行验证。')
  if (config.value?.dnsEnabled === false) hints.push('Mihomo DNS 未开启，请确认设备使用的 DNS 服务可达。')
  return hints
})
const observed = computed(() => lanDevices.value.some(device => device.tunCount > 0))
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
    if (!h.value.interfaces.filter(i => !isContainerInterface(i.name)).some(i => `${i.name}/${i.address}` === selected.value)) {
      const first = h.value.interfaces.find(i => !isContainerInterface(i.name)); selected.value = first ? `${first.name}/${first.address}` : ''
    }
  } else { host.value = null; hostError.value = errorMessage(h.reason) }
  if (r.status === 'fulfilled') { runtimeTun.value = typeof r.value.configs?.tun?.enable === 'boolean' ? r.value.configs.tun.enable : null; runtimeError.value = '' }
  else { runtimeTun.value = null; runtimeError.value = errorMessage(r.reason) }
  if (c.status === 'fulfilled') { connections.value = c.value.connections || []; connectionError.value = '' }
  else { connections.value = []; connectionError.value = errorMessage(c.reason) }
  devices.value = bypassInventory.update(connections.value, host.value?.neighbors || [], host.value?.interfaces || [], Date.now(), !connectionError.value && !hostError.value)
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
    <div class="monitor-overview">
      <div class="monitor-summary" role="status"><strong>{{ t(connectionError || hostError ? '流量状态待验证' : observed ? '已观察到局域网 TUN 连接' : '尚未观察到局域网 TUN 连接') }}</strong><span>{{ t(refreshing ? '检查中…' : '只读检查') }}<template v-if="updatedAt"> · {{ updatedAt }}</template></span></div>
      <details v-if="diagnosticHints.length" class="diagnostic-hints"><summary>{{ t('需要关注') }} · {{ diagnosticHints.length }}<span>{{ t(diagnosticHints[0] || '') }}</span></summary><p v-for="hint in diagnosticHints" :key="hint">{{ t(hint) }}</p><small>{{ t('以上为配置提示，不是设备失败日志；具体结果需要结合连接与日志验证。') }}</small></details>
      <p v-if="host?.legacyJournal" class="warn-text" role="alert">{{ t('检测到旧开发版的旁路由恢复记录，请先使用旧版本停止并恢复；本页不会自动修改网络。') }}</p>
    </div>
    <div class="card monitor-devices">
      <div class="device-toolbar">
        <div class="device-heading"><h3>{{ t('局域网设备') }}</h3><span class="tag">{{ visibleDevices.length }}</span></div>
        <div class="network-filter">
          <label><input v-model="onlyConnected" type="checkbox">{{ t('仅显示已连接') }}</label>
          <label :title="t('默认隐藏 Docker 内部网桥；直接接入局域网的容器仍会显示。')"><input v-model="showContainers" type="checkbox">{{ t('显示容器网络') }}<span class="muted">({{ containerCount }})</span></label>
        </div>
      </div>
      <p v-if="host?.neighborStatus !== 'available'" class="muted">{{ t('邻居表不可读取，当前仅展示连接来源和本页会话记录。') }}</p>
      <p v-if="connectionError" class="warn-text" role="alert">{{ t('连接数据不可用，当前接管状态待验证。') }}</p>
      <div class="monitor-table"><table><thead><tr><th>{{ t('设备 IP') }}</th><th>{{ t('连接状态') }}</th><th>{{ t('当前连接') }}</th><th>{{ t('当前连接累计流量') }}</th><th>{{ t('操作') }}</th></tr></thead><tbody><tr v-for="device in visibleDevices" :key="device.ip"><td><strong>{{ device.ip }}</strong><small v-if="isContainerInterface(device.interface)">{{ t('容器网络') }}</small></td><td><span class="tag connection-state" :class="{ connected: !connectionError && device.recentTraffic }">{{ connectionError ? '—' : t(device.recentTraffic ? '已连接' : '未连接') }}</span></td><td>{{ connectionError ? '—' : device.count }}</td><td><span v-if="connectionError">—</span><span v-else class="device-traffic"><span>↑ {{ formatBytes(device.upload) }}</span><span class="device-download">↓ {{ formatBytes(device.download) }}</span></span></td><td><button type="button" class="ghost device-details-button" aria-haspopup="dialog" @click="selectedDevice = device.ip">{{ t('查看详情') }}</button></td></tr><tr v-if="!visibleDevices.length"><td colspan="5" class="device-empty" role="status">{{ t(connectionError ? '连接数据不可用，当前接管状态待验证。' : onlyConnected ? '暂无已连接设备，可取消筛选查看全部设备。' : '尚未发现其他局域网设备。请让设备发起访问后刷新；空列表不代表不支持旁路由。') }}</td></tr></tbody></table></div>
      <details class="device-help"><summary>{{ t('设备列表说明') }}</summary>
      <p class="muted">{{ t('每 10 秒刷新，仅代表当前可见连接；不会修改网络设置。') }}</p>
      <p class="muted">{{ t('最近 60 秒观察到上传或下载增长显示已连接，否则显示未连接；首次采样仅建立基线，状态不代表设备在线或已由 TUN 接管。') }}</p>
      <p class="muted">{{ t('邻居表可能不完整。最近观察表示看到邻居记录或连接的时间，不是设备最后在线时间。本页会话最多保留 256 个设备、24 小时；刷新浏览器后清除。') }}</p>
      </details>
      <a href="#connections">{{ t('查看全部连接') }}</a>
    </div>
    <SequenceFormDialog :open="!!activeDevice" :title="`${t('设备诊断')} · ${activeDevice?.ip || ''}`" hide-footer wide @close="selectedDevice = ''">
      <BypassDeviceDetails v-if="activeDevice" :key="activeDevice.ip" :ip="activeDevice.ip" :device="activeDevice" :connections="connections" :traffic-available="!connectionError" />
    </SequenceFormDialog>
    <section class="card monitor-details">
      <div class="detail-tabs" role="tablist" :aria-label="t('旁路由检查')">
        <button v-for="(tab, index) in detailTabs" :id="`bypass-tab-${tab.key}`" :key="tab.key" type="button" role="tab" :aria-selected="detailTab === tab.key" :aria-controls="`bypass-panel-${tab.key}`" :tabindex="detailTab === tab.key ? 0 : -1" @click="detailTab = tab.key" @keydown="navigateDetailTab($event, index)">{{ t(tab.label) }}</button>
      </div>
      <div v-show="detailTab === 'checks'" id="bypass-panel-checks" class="monitor-checks" role="tabpanel" aria-labelledby="bypass-tab-checks" tabindex="0">

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
      <div v-show="detailTab === 'guide'" id="bypass-panel-guide" class="monitor-guide" role="tabpanel" aria-labelledby="bypass-tab-guide" tabindex="0">
      <div v-if="lanInterfaces.length" class="guide-interface"><span>{{ t('接入网卡') }}</span><AppSelect v-model="selected" :label="t('接入网卡')" :options="lanInterfaces.map(i => ({ value: `${i.name}/${i.address}`, label: `${i.name} · ${i.address}` }))" /></div>
      <dl class="guide-address">
        <div><dt>{{ t('默认网关') }}</dt><dd>{{ selectedInterface?.address || '—' }}</dd></div>
        <div><dt>{{ t('局域网网段') }}</dt><dd>{{ selectedInterface?.cidr || '—' }}</dd></div>
        <div><dt>DNS <HelpPopover label="DNS"><span>{{ t('设备默认网关可填写所选 NAS IPv4；DNS 仅在 NAS 已提供可达的 DNS 服务时指向该地址。') }}</span></HelpPopover></dt><dd>{{ t('确认 NAS DNS 可用后再填写') }}</dd></div>
      </dl>
      <div class="guide-verify"><span>{{ t('访问网站后，在设备详情中查看 TUN 连接。') }}</span><HelpPopover :label="t('设备接入')"><span>{{ t('在设备上访问网站，然后观察此页是否出现该设备源 IP 的 TUN 连接、命中规则和出口链。') }}</span><span>{{ t('IPv6、同网段直连和被排除的目标可能绕过 TUN；本页不证明这些流量已接管。') }}</span></HelpPopover></div>
      </div>
    </section>
  </div>
</template>
<style scoped>
.network-filter { display:flex;flex-wrap:wrap;gap:8px 16px;align-items:center;margin:0;font-size:12px; }
.network-filter label { display:flex;align-items:center;gap:6px;white-space:nowrap; }
.network-filter input { width:14px;height:14px;min-height:0;margin:0; }
summary { cursor:pointer;font-weight:600;padding:2px 0; }
.monitor-overview { padding:2px 0 0;min-width:0; }
.device-toolbar { display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:10px 16px;margin-bottom:12px; }
.device-heading { display:flex;align-items:center;gap:8px; }
.device-heading h3 { margin:0;font-size:15px; }
.device-help { margin:10px 0;font-size:12px;color:var(--muted); }
.connection-state { color:var(--muted); }
.connection-state.connected { color:var(--good);background:color-mix(in srgb,var(--good) 10%,transparent);border-color:color-mix(in srgb,var(--good) 25%,transparent); }
.diagnostic-hints { margin-top:8px; }
.monitor-summary { display:flex;flex-wrap:wrap;align-items:baseline;justify-content:space-between;gap:6px 16px;font-size:13px; }
.monitor-summary>span { font-size:12px;white-space:nowrap; }
.diagnostic-hints { padding:9px 12px;border:1px solid var(--line);border-radius:8px;font-size:12px; }
.diagnostic-hints summary { font-size:12px;line-height:1.6; }
.diagnostic-hints summary span { margin-left:12px;color:var(--muted);font-weight:400; }
.diagnostic-hints[open] summary span { display:none; }
.diagnostic-hints p { margin:6px 0;font-size:13px; }
.monitor-table button { white-space:nowrap; }
.device-traffic { display:inline-flex;align-items:center;gap:12px;white-space:nowrap;font-variant-numeric:tabular-nums; }
.device-download { color:var(--muted); }
.monitor-table .device-details-button { min-height:26px;padding:3px 9px;border-radius:6px;font-size:12px;font-weight:500;line-height:18px; }
.bypass-monitor { display: grid; gap: 10px; }
.monitor-details { padding:12px 16px;min-width:0; }
.monitor-checks, .monitor-guide { min-width:0;font-size:13px; }
.detail-tabs { display:flex;gap:4px;padding-bottom:10px;border-bottom:1px solid var(--line); }
.detail-tabs button { border:1px solid var(--line);background:transparent;color:var(--muted);box-shadow:none;border-radius:7px;padding:6px 12px;font-size:12px;line-height:18px; }
.detail-tabs button:hover { border-color:var(--accent);color:var(--text); }
.detail-tabs button[aria-selected="true"] { border-color:color-mix(in srgb,var(--accent) 48%,var(--line));background:color-mix(in srgb,var(--accent) 14%,var(--panel2));color:var(--accent); }
.detail-tabs button:focus-visible { outline:2px solid var(--accent);outline-offset:2px; }
.monitor-table .device-empty { padding:28px 16px;text-align:center;color:var(--muted); }
.monitor-details a, .monitor-devices>a { color:var(--accent);font-size:12px;text-decoration:none; }
.monitor-details a:hover, .monitor-devices>a:hover { text-decoration:underline; }
.monitor-devices { min-width:0;padding:12px 16px; }
.monitor-summary span, .monitor-summary small { color:var(--muted); }
.check-list { margin:4px 0 10px; }
.check-list > div { display:grid;grid-template-columns:minmax(140px,1fr) 2fr;gap:12px;padding:8px 0;border-bottom:1px solid var(--line); }
.check-list dd, .guide-address dd { margin:0;overflow-wrap:anywhere; }
small { display:block;color:var(--muted);margin-top:5px; }
.monitor-table { overflow-x:auto; }
.monitor-table td { max-width:300px;padding-top:10px;padding-bottom:10px;overflow-wrap:anywhere;vertical-align:top; }
.monitor-table th { white-space:nowrap; }
.guide-interface { display:grid;grid-template-columns:140px minmax(0,340px);align-items:center;gap:12px;margin:14px 0; }
.guide-address { display:flex;gap:16px 36px;flex-wrap:wrap;margin:12px 0; }
.guide-address dt { display:flex;align-items:center;gap:6px;color:var(--muted);font-size:12px; }
.guide-verify { display:flex;align-items:center;gap:6px;color:var(--muted);font-size:12px;margin-top:16px; }
.guide-address dd { margin-top:6px; }
p { line-height:1.65; }
@media(max-width:600px) { .check-list > div,.guide-interface { grid-template-columns:1fr;gap:6px; } }
</style>
