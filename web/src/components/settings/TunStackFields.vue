<script setup lang="ts">
import { computed } from 'vue'
import HelpPopover from '@/components/HelpPopover.vue'
import type { TunFeatures, TunSetting } from '@/types/api'

const props = defineProps<{ features?: TunFeatures; disabled?: boolean; fieldClass?: string }>()
const stack = defineModel<NonNullable<TunSetting['stack']>>('stack', { required: true })
const congestionController = defineModel<NonNullable<TunSetting['congestionController']>>('congestionController', { required: true })
const emit = defineEmits<{ change: [key: 'stack' | 'congestionController'] }>()
const defaultLabel = computed(() => props.features?.defaultStack ? `跟随内核默认（${props.features.defaultStack}）` : '跟随内核默认')
const usesMips = computed(() => stack.value === 'mips' || (!stack.value && props.features?.defaultStack === 'mips'))
</script>

<template>
  <div class="field" :class="fieldClass">
    <div class="field-label-row">
      <label>TUN 协议栈</label>
      <HelpPopover label="TUN 协议栈"><strong>协议栈决定内核如何处理 TUN 流量</strong><span><b>跟随内核默认</b>：不指定协议栈，1.19.32 起默认使用 mips。</span><span><b>mips</b>：新版用户态协议栈，支持拥塞控制设置。</span><span><b>mixed</b>：TCP 使用 System，UDP 使用 gVisor。</span><span><b>system</b>：使用系统协议栈。</span><span><b>gVisor</b>：使用 gVisor 用户态协议栈。</span></HelpPopover>
    </div>
    <select v-model="stack" aria-label="TUN 协议栈" :disabled="disabled" @change="emit('change', 'stack')">
      <option value="">{{ defaultLabel }}</option>
      <option v-if="features?.mips || stack === 'mips'" value="mips" :disabled="!features?.mips">mips</option>
      <option value="mixed">mixed</option><option value="system">system</option><option value="gvisor">gVisor</option>
    </select>
  </div>
  <div v-if="features?.congestionController && usesMips" class="field" :class="fieldClass">
    <div class="field-label-row">
      <label>TCP 拥塞控制</label>
      <HelpPopover label="TCP 拥塞控制"><strong>高级选项，仅用于 mips 协议栈</strong><span>建议保持内核默认；显式选择的算法用于 TUN 内的 TCP 连接。</span></HelpPopover>
    </div>
    <select v-model="congestionController" aria-label="TCP 拥塞控制" :disabled="disabled" @change="emit('change', 'congestionController')">
      <option value="">内核默认</option><option value="cubic">CUBIC</option><option value="reno">Reno</option><option value="bbr">BBR</option><option value="bbr3">BBRv3</option>
    </select>
  </div>
</template>
