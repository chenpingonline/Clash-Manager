<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { errorMessage } from '@/services/api'
import { bypassRequest } from '@/services/bypass-setup'
import { t } from '@/services/i18n'
import type { LanInterface, MonitorHost } from '@/services/bypass-monitor'
import { isContainerInterface } from '@/services/bypass-monitor'
import AppSelect from '@/components/AppSelect.vue'
import SequenceFormDialog from '@/components/SequenceFormDialog.vue'
const props = defineProps<{ host: MonitorHost | null }>()
const emit = defineEmits<{ changed: [] }>()
const open = ref(false), mode = ref<'redir' | 'tproxy'>('tproxy'), selected = ref(''), busy = ref(false), error = ref(''), message = ref('')
const plan = ref<{ mode: string; interface: LanInterface; port: number } | null>(null)
const interfaces = computed(() => (props.host?.interfaces || []).filter(item => !isContainerInterface(item.name)))
const iface = computed(() => interfaces.value.find(item => `${item.name}/${item.address}` === selected.value))
watch(interfaces, items => { if (!items.some(item => `${item.name}/${item.address}` === selected.value)) selected.value = items[0] ? `${items[0].name}/${items[0].address}` : '' }, { immediate: true })
watch([mode, selected], () => { plan.value = null; error.value = '' })
function payload() { return { mode: mode.value, interface: iface.value?.name, address: iface.value?.address } }
async function preview() {
  busy.value = true; error.value = ''; message.value = ''; plan.value = null
  try { plan.value = await bypassRequest<{ mode: string; interface: LanInterface; port: number }>('preview', payload()) }
  catch (cause) { error.value = errorMessage(cause) }
  finally { busy.value = false }
}
async function apply() {
  if (!plan.value || busy.value) return
  busy.value = true; error.value = ''
  try {
    await bypassRequest('configure', { ...payload(), confirmed: true })
    message.value = 'IPv4 接管规则已配置，请在设备上发起访问验证。'; open.value = false; plan.value = null; emit('changed')
  } catch (cause) { error.value = errorMessage(cause); emit('changed') }
  finally { busy.value = false }
}
async function stop() {
  if (busy.value) return
  busy.value = true; error.value = ''; message.value = ''
  try { await bypassRequest('stop', {}); message.value = '接管规则已停用，透明代理端口配置保留。'; emit('changed') }
  catch (cause) { error.value = errorMessage(cause) }
  finally { busy.value = false }
}
</script>
<template>
  <section class="card bypass-setup">
    <div class="bypass-setup-row"><div><strong>{{ t('一键配置') }}</strong><p>{{ props.host?.managed?.active ? `${props.host.managed.mode === 'tproxy' ? 'TProxy' : 'Redir'} · ${props.host.managed.interface?.name} · ${props.host.managed.interface?.cidr}` : t('为所选局域网网卡配置 IPv4 透明代理接管') }}</p></div><button v-if="props.host?.managed?.active" class="ghost" :disabled="busy" @click="stop">{{ t('停用并恢复') }}</button><button v-else class="ghost" :disabled="busy || !props.host?.setupSupported" @click="open = true; error = ''; plan = null">{{ t('配置接管规则') }}</button></div>
    <p v-if="props.host && !props.host.setupSupported" class="muted">{{ t('一键配置需要 Linux、nftables、iproute2 和网络管理权限。') }}</p>
    <p v-if="props.host?.managed?.active && props.host.managed.rulesPresent !== true" class="warn-text">{{ t('接管规则状态待验证') }}</p>
    <p v-if="props.host?.managed?.error" class="warn-text">{{ props.host.managed.error }}</p>
    <p v-if="message" role="status">{{ t(message) }}</p><p v-if="error && !open" class="warn-text" role="alert">{{ error }}</p>
    <SequenceFormDialog :open="open" :title="t('配置接管规则')" @close="!busy && (open = false)">
      <div class="bypass-setup-form">
        <label>{{ t('接管模式') }}<AppSelect v-model="mode" :label="t('接管模式')" :disabled="busy" :options="[{ value: 'tproxy', label: 'TProxy · TCP / UDP' }, { value: 'redir', label: 'Redir · TCP' }]" /></label>
        <label>{{ t('接入网卡') }}<AppSelect v-model="selected" :label="t('接入网卡')" :disabled="busy" :options="interfaces.map(item => ({ value: `${item.name}/${item.address}`, label: `${item.name} · ${item.address} · ${item.cidr}` }))" /></label>
        <p>{{ t('仅支持运行中的托管内核；请先关闭 TUN，避免重复接管。') }}</p>
        <p>{{ t('仅接管所选网卡的 IPv4 公网流量；保留 DNS、IPv6 和内网目标的原有路径。') }}</p>
        <p>{{ t('将启用对应透明代理端口，并把代理监听绑定到所选网卡地址；其他已启用代理端口的可访问范围也会随之变化。') }}</p>
        <p>{{ t('配置独立防火墙表和必要的策略路由，开启 IPv4 转发；停用时恢复本应用修改的系统规则，保留内核端口设置。') }}</p>
        <p>{{ t('内核停止、重启、配置重新应用或应用退出后会清理接管规则，需要时重新启用。设备网关仍需手动指向所选 NAS 地址。') }}</p>
        <p v-if="error" class="warn-text" role="alert">{{ error }}</p>
        <div v-if="plan" class="bypass-plan"><strong>{{ t('预检查通过') }}</strong><p>{{ plan.mode === 'tproxy' ? 'TProxy' : 'Redir' }} · {{ plan.interface.name }} · {{ plan.interface.cidr }} · {{ plan.port }}</p><p>{{ t('配置成功仅代表规则已安装，不代表设备全部流量已被接管。') }}</p></div>

      </div>
      <template #footer><div class="bypass-setup-actions"><button class="ghost" :disabled="busy || !iface" @click="preview">{{ t('预检查') }}</button><button :disabled="busy || !plan" @click="apply">{{ t(busy ? '处理中…' : '确认配置') }}</button></div></template>
    </SequenceFormDialog>
  </section>
</template>
<style scoped>
.bypass-setup { min-width:0;padding:12px 16px; }
.bypass-setup-row { display:flex;align-items:center;justify-content:space-between;gap:12px; }
.bypass-setup strong { font-size:13px; }
.bypass-setup p { margin:5px 0 0;color:var(--muted);font-size:12px;line-height:1.6;overflow-wrap:anywhere; }
.bypass-setup button { flex:none;font-size:12px; }
.bypass-setup-form { display:grid;gap:10px; }
.bypass-setup-form p { margin:0;color:var(--muted);font-size:12px;line-height:1.55; }
.bypass-setup-form label { display:grid;gap:6px;font-size:12px; }
.bypass-plan { padding:10px;border:1px solid var(--line);border-radius:8px;background:var(--panel2); }
.bypass-setup .warn-text, .bypass-setup-form .warn-text { color:var(--warn); }
.bypass-setup-actions { display:flex;width:100%;justify-content:flex-end;gap:8px; }
</style>
