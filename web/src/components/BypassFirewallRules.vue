<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { api, errorMessage, isAbortError } from '@/services/api'
import { t } from '@/services/i18n'
import type { MonitorHost } from '@/services/bypass-monitor'
import SequenceFormDialog from '@/components/SequenceFormDialog.vue'
defineProps<{ firewall?: MonitorHost['firewall'] }>()
const open = ref(false), loading = ref(false), error = ref('')
const rules = ref<{ text: string; truncated: boolean; readAt: string } | null>(null)
let request: AbortController | undefined
function cancel() { request?.abort(); request = undefined; loading.value = false }
function close() { open.value = false; cancel(); rules.value = null; error.value = '' }
async function load() {
  cancel()
  const active = new AbortController(); request = active; loading.value = true; error.value = ''
  const timeout = setTimeout(() => active.abort(), 8000)
  try {
    const result = await api<{ text: string; truncated: boolean; readAt: string }>('/api/network/bypass/rules', { signal: active.signal, cache: 'no-store' })
    if (request === active && open.value) rules.value = result
  } catch (cause) {
    if (request === active && open.value) error.value = isAbortError(cause) ? t('读取防火墙规则超时，请重试') : errorMessage(cause)
  } finally {
    clearTimeout(timeout)
    if (request === active) { loading.value = false; request = undefined }
  }
}
function show() { open.value = true; rules.value = null; void load() }
onBeforeUnmount(cancel)
</script>
<template>
  <details class="advanced-rules">
    <summary>{{ t('高级规则信息') }}</summary>
    <div class="firewall-summary"><div><strong>{{ t('防火墙与 NAT') }}</strong><p>{{ t(firewall?.readable ? '已读取，转发效果待验证' : '无法读取，待验证') }}</p><p v-if="firewall?.readable">{{ t('转发链') }} {{ firewall.forwardChains }} · {{ t('源 NAT 规则') }} {{ firewall.natRules }} · {{ t('丢弃规则') }} {{ firewall.dropRules }}</p></div><button class="ghost" aria-haspopup="dialog" @click="show">{{ t('查看规则明细') }}</button></div>
    <p>{{ t('规则统计包含系统与其他服务；丢弃规则数量不是丢包数量，也不表示必须额外添加 NAT。') }}</p>
  </details>
  <SequenceFormDialog :open="open" :title="t('防火墙规则明细')" wide hide-footer @close="close">
    <p class="rules-scope">{{ t('只读显示当前系统的 nftables 规则，包含系统与其他服务；规则名称不能证明归属或转发效果。') }}</p>
    <div class="rules-toolbar"><button class="ghost" :disabled="loading" @click="load">{{ t('刷新规则') }}</button><span v-if="rules" class="muted">{{ t('读取时间') }} · {{ new Date(rules.readAt).toLocaleString() }}</span><span v-if="loading" role="status">{{ t('读取中…') }}</span></div>
    <p v-if="error" class="warn-text" role="alert">{{ t(error) }}</p>
    <p v-if="rules?.truncated" class="warn-text" role="status">{{ t('规则内容过大，仅显示前 2 MiB。') }}</p>
    <pre v-if="rules?.text" class="rules-text" tabindex="0" :aria-label="t('nftables 规则内容')">{{ rules.text }}</pre>
    <p v-else-if="rules && !loading">{{ t('当前 nftables 规则集为空。') }}</p>
  </SequenceFormDialog>
</template>
<style scoped>
.advanced-rules { margin:12px 0;padding:10px 12px;border:1px solid var(--line);border-radius:8px;font-size:12px; }
summary { cursor:pointer;font-weight:600; }
.firewall-summary { display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:12px;margin-top:12px; }
p { margin:6px 0;color:var(--muted);font-size:12px;line-height:1.6; }
button { font-size:12px; }
.rules-toolbar { display:flex;align-items:center;flex-wrap:wrap;gap:12px;margin:12px 0;font-size:12px; }
.rules-text { margin:0;padding:12px;max-height:48dvh;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;border:1px solid var(--line);border-radius:8px;background:var(--panel2);font-family:var(--font-mono);font-size:12px;line-height:1.6; }
p.warn-text { color:var(--warn); }
</style>
