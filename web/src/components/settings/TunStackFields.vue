<script setup lang="ts">
import AppSelect from '@/components/AppSelect.vue'
import { t } from '@/services/i18n'

import { computed, useId } from 'vue'
import HelpPopover from '@/components/HelpPopover.vue'
import type { TunFeatures, TunSetting } from '@/types/api'

const props = defineProps<{ features?: TunFeatures; disabled?: boolean; fieldClass?: string }>()
const stack = defineModel<NonNullable<TunSetting['stack']>>('stack', { required: true })
const congestionController = defineModel<NonNullable<TunSetting['congestionController']>>('congestionController', { required: true })
const emit = defineEmits<{ change: [key: 'stack' | 'congestionController'] }>()
const defaultLabel = computed(() => props.features?.defaultStack ? `跟随内核默认（${props.features.defaultStack}）` : '跟随内核默认')
const stackId = `tun-stack-${useId()}`
const congestionId = `tun-congestion-${useId()}`
const usesMips = computed(() => stack.value === 'mips' || (!stack.value && props.features?.defaultStack === 'mips'))
</script>

<template>
  <div class="field" :class="fieldClass">
    <div class="field-label-row">
      <label :for="stackId">{{ t("TUN 协议栈") }}</label>
      <HelpPopover :label="t('TUN 协议栈')"><strong>{{ t("协议栈决定内核如何处理 TUN 流量") }}</strong><span><b>{{ t("跟随内核默认") }}</b>{{ t("：不指定协议栈，1.19.32 起默认使用 mips。") }}</span><span><b>mips</b>{{ t("：新版用户态协议栈，支持拥塞控制设置。") }}</span><span><b>mixed</b>{{ t("：TCP 使用 System，UDP 使用 gVisor。") }}</span><span><b>system</b>{{ t("：使用系统协议栈。") }}</span><span><b>gVisor</b>{{ t("：使用 gVisor 用户态协议栈。") }}</span></HelpPopover>
    </div>
    <AppSelect :id="stackId" v-model="stack" :aria-label="t('TUN 协议栈')" :disabled="disabled" @change="emit('change', 'stack')" :options="[{ value: '', label: (t(defaultLabel)) }, ...(features?.mips || stack === 'mips' ? [{ value: 'mips', label: 'mips', disabled: !features?.mips }] : []), { value: 'mixed', label: 'mixed' }, { value: 'system', label: 'system' }, { value: 'gvisor', label: 'gVisor' }]" />
  </div>
  <div v-if="features?.congestionController && usesMips" class="field" :class="fieldClass">
    <div class="field-label-row">
      <label :for="congestionId">{{ t("TCP 拥塞控制") }}</label>
      <HelpPopover :label="t('TCP 拥塞控制')"><strong>{{ t("高级选项，仅用于 mips 协议栈") }}</strong><span>{{ t("建议保持内核默认；显式选择的算法用于 TUN 内的 TCP 连接。") }}</span></HelpPopover>
    </div>
    <AppSelect :id="congestionId" v-model="congestionController" :aria-label="t('TCP 拥塞控制')" :disabled="disabled" @change="emit('change', 'congestionController')" :options="[{ value: '', label: (t('内核默认')) }, { value: 'cubic', label: 'CUBIC' }, { value: 'reno', label: 'Reno' }, { value: 'bbr', label: 'BBR' }, { value: 'bbr3', label: 'BBRv3' }]" />
  </div>
</template>
