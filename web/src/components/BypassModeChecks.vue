<script setup lang="ts">
import { t } from '@/services/i18n'
import type { BypassMode, MonitorConfig } from '@/services/bypass-monitor'
const props = defineProps<{ mode: BypassMode; config?: MonitorConfig | null; runtimeTun: boolean | null; runtimeError?: string; unavailable: boolean }>()
function setting(value: boolean | null | undefined) {
  return props.unavailable ? '配置不可读取' : value === true ? '已开启' : value === false ? '未开启' : '未显式配置'
}
</script>
<template>
  <dl class="mode-check-list">
    <template v-if="mode === 'TUN'">
      <div><dt>{{ t('TUN 配置') }}</dt><dd>{{ t(setting(config?.tun)) }}</dd></div>
      <div><dt>{{ t('TUN 运行态') }}</dt><dd>{{ t(runtimeTun === true ? '已开启' : runtimeTun === false ? '未开启' : '运行态无法判断') }}<small v-if="runtimeError">{{ t('Controller 不可用，无法读取实际运行态。') }}</small></dd></div>
      <div><dt>Auto Route</dt><dd>{{ t(setting(config?.autoRoute)) }}</dd></div>
      <div><dt>Auto Redirect</dt><dd>{{ t(setting(config?.autoRedirect)) }}</dd></div>
      <div><dt>{{ t('DNS 劫持配置') }}</dt><dd>{{ unavailable ? t('劫持配置无法判断') : config?.dnsHijack?.join(', ') || t('未配置 DNS 劫持') }}</dd></div>
      <div><dt>{{ t('TUN 排除网段') }}</dt><dd>{{ unavailable ? t('配置不可读取') : config?.routeExclude?.join(', ') || t('未配置排除网段') }}</dd></div>
    </template>
    <div v-else><dt>{{ mode }} {{ t('配置端口') }}</dt><dd>{{ unavailable ? t('配置不可读取') : (mode === 'Redir' ? config?.redirPort : config?.tproxyPort) || t('未配置') }}<small>{{ t('端口来自配置，不代表已监听或已接管流量。') }}</small></dd></div>
  </dl>
</template>
<style scoped>
.mode-check-list { margin:0; }
.mode-check-list > div { display:grid;grid-template-columns:minmax(140px,1fr) 2fr;gap:12px;padding:8px 0;border-bottom:1px solid var(--line); }
dd { margin:0;overflow-wrap:anywhere; }
small { display:block;color:var(--muted);margin-top:5px; }
@media(max-width:600px) { .mode-check-list > div { grid-template-columns:1fr;gap:6px; } }
</style>
