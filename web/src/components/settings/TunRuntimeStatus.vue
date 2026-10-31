<script setup lang="ts">
import { useTunState } from '@/composables/useTunState'
import { t } from '@/services/i18n'
const { tunState } = useTunState()
</script>
<template>
  <section class="tun-runtime-status" :class="tunState.phase" :aria-label="t('TUN 实际运行状态')" role="status" aria-live="polite">
    <strong>{{ t(tunState.message) }}</strong>
    <div class="tun-runtime-facts">
      <span>{{ t('保存的开启设置') }}：{{ t(tunState.desiredEnabled === undefined ? '未知' : tunState.desiredEnabled ? '已开启' : '已关闭') }}</span>
      <span>{{ t('Core 运行状态') }}：{{ t(tunState.runtimeEnabled === undefined ? '未知' : tunState.runtimeEnabled ? '已开启' : '已关闭') }}</span>
      <span v-if="tunState.interface?.name">{{ t('实际 TUN 网卡') }}：{{ tunState.interface.name }} · {{ t(!tunState.interface.checked ? '无法确认' : tunState.interface.exists && tunState.interface.tun && tunState.interface.up ? '已就绪' : '未就绪') }}</span>
    </div>
    <p v-for="issue in tunState.requirements?.issues || []" :key="issue" class="warn-text">{{ t(issue) }}</p>
    <p v-for="warning in tunState.requirements?.warnings || []" :key="warning" class="hint">{{ t(warning) }}</p>
  </section>
</template>
<style scoped>
.tun-runtime-status { padding: 10px 12px; margin: 10px 0; border: 1px solid var(--line); border-radius: 8px; font-size: 12px; line-height: 1.6; }
.tun-runtime-status.enabled strong { color: var(--good); }
.tun-runtime-status.mismatch strong, .tun-runtime-status.unknown strong { color: var(--warn); }
.tun-runtime-facts { display: flex; flex-wrap: wrap; gap: 4px 16px; color: var(--muted); }
p { margin: 5px 0 0; }
</style>
