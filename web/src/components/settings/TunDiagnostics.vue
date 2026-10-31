<script setup lang="ts">
import { onMounted, ref } from 'vue'
import HelpPopover from '@/components/HelpPopover.vue'
import { api, errorMessage, jsonRequest } from '@/services/api'
import { t } from '@/services/i18n'
import { copyText } from '@/services/clipboard'
import type { NetworkSettingsResponse, TunDiagnostics } from '@/types/api'
const props = defineProps<{ busy: boolean; supported: boolean; capabilityText: string }>()
const emit = defineEmits<{ capability: [value: NonNullable<NetworkSettingsResponse['tunCapability']>] }>()
const diagnostic = ref<TunDiagnostics>(), working = ref(false), error = ref(''), copied = ref(false)
const copyError = ref('')
async function check(repair = false) {
  if (working.value || props.busy) return
  working.value = true; error.value = ''; copied.value = false; copyError.value = ''
  try {
    const result = await api<TunDiagnostics>(`/api/network/tun/${repair ? 'repair' : 'diagnostics'}`, repair ? jsonRequest('POST', {}) : undefined)
    diagnostic.value = result
    emit('capability', result.capability)
    if (result.error) error.value = result.error
  } catch (cause) { error.value = errorMessage(cause) }
  finally { working.value = false }
}
async function copy() {
  if (!diagnostic.value) return
  copied.value = false; copyError.value = ''
  try { await copyText(JSON.stringify(diagnostic.value, null, 2)); copied.value = true }
  catch { copyError.value = '复制失败，请展开诊断信息手动复制' }
}
onMounted(() => { void check() })
</script>
<template>
  <section class="tun-diagnostics" :aria-label="t('TUN 诊断与修复')">
    <div class="tun-capability" :class="supported ? 'ok' : 'warn'">
      <strong>{{ t(supported ? '可用' : '不可用') }}</strong>
      <span class="capability-message">{{ t(capabilityText) }}</span>
      <div class="diagnostic-actions">
      <button class="ghost" type="button" :disabled="working || busy" @click="check()">{{ t(working ? '正在检测或修复…' : '重新检测') }}</button>
      <span v-if="diagnostic?.repairable" class="repair-action">
        <button type="button" :disabled="working || busy" @click="check(true)">{{ t('尝试修复 TUN') }}</button>
        <HelpPopover :label="t('尝试修复 TUN')" :teleport="false" @keydown.esc.stop><span>{{ t('修复只加载 TUN 驱动并补建缺失的设备节点，不会开启 TUN 或修改路由。容器需在宿主配置设备透传与 NET_ADMIN 权限。') }}</span></HelpPopover>
      </span>
      <button v-if="diagnostic" class="ghost" type="button" :disabled="working" @click="copy">{{ t(copied ? '已复制' : '复制诊断信息') }}</button>
      </div>
    </div>
    <slot />
    <p v-if="error" class="bad-text" role="alert">{{ t(error) }}</p>
    <p v-if="copyError" class="bad-text" role="alert">{{ t(copyError) }}</p>
    <details v-if="diagnostic"><summary>{{ t('诊断信息') }}</summary><pre tabindex="0">{{ JSON.stringify(diagnostic, null, 2) }}</pre></details>
  </section>
</template>
<style scoped>
.tun-diagnostics { margin: 0 0 12px; }
.tun-capability { flex-wrap: wrap; }
.capability-message { flex: 1; min-width: 160px; }
.diagnostic-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin-left: auto; }
.repair-action { display: inline-flex; align-items: center; gap: 5px; }
.diagnostic-actions > button, .repair-action > button { min-height: 26px; height: 26px; padding: 0 9px; border-radius: 7px; font-size: 12px; font-weight: 400; line-height: 1.2; }
pre { white-space: pre-wrap; overflow-wrap: anywhere; font-size: 12px; max-height: 260px; overflow: auto; padding: 12px; background: var(--frame); border-radius: 8px; user-select: text; }
summary { cursor: pointer; font-size: 12px; }
</style>
