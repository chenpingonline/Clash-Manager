<script setup lang="ts">
import { t } from '@/services/i18n'

import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { NormalizedRule } from '@/services/rules'

const props = defineProps<{ items: NormalizedRule[]; busy: boolean; query: string }>()
const emit = defineEmits<{ toggle: [rule: NormalizedRule] }>()

const rowHeight = 52
const overscan = 8
const viewport = ref<HTMLElement | null>(null)
const scrollTop = ref(0)
const viewportHeight = ref(520)
let observer: ResizeObserver | null = null

const start = computed(() => Math.max(0, Math.floor(scrollTop.value / rowHeight) - overscan))
const end = computed(() => Math.min(props.items.length, Math.ceil((scrollTop.value + viewportHeight.value) / rowHeight) + overscan))
const visible = computed(() => props.items.slice(start.value, end.value).map((rule, offset) => ({ rule, index: start.value + offset })))

function onScroll() {
  scrollTop.value = viewport.value?.scrollTop || 0
}

function proxyClass(proxy: string) {
  const value = proxy.toLocaleLowerCase()
  if (value === 'direct') return 'direct'
  if (value === 'reject' || value === 'reject-drop') return 'reject'
  return 'policy'
}

watch(() => props.query, () => {
  scrollTop.value = 0
  if (viewport.value) viewport.value.scrollTop = 0
})

watch(() => props.items.length, count => {
  const next = Math.min(scrollTop.value, Math.max(0, count * rowHeight - viewportHeight.value))
  scrollTop.value = next
  if (viewport.value) viewport.value.scrollTop = next
})

onMounted(() => {
  if (!viewport.value) return
  viewportHeight.value = viewport.value.clientHeight
  observer = new ResizeObserver(entries => {
    viewportHeight.value = entries[0]?.contentRect.height || viewport.value?.clientHeight || 520
  })
  observer.observe(viewport.value)
})
onBeforeUnmount(() => observer?.disconnect())
</script>

<template>
  <div class="rules-table" role="table" :aria-label="t('当前生效规则')" :aria-rowcount="items.length">
    <div class="rule-table-head" role="row">
      <span role="columnheader">{{ t("序号") }}</span><span role="columnheader">{{ t("规则内容") }}</span><span role="columnheader">{{ t("类型") }}</span><span role="columnheader">{{ t("目标策略") }}</span>
      <span role="columnheader">{{ t("启用") }}</span>
    </div>
    <div ref="viewport" class="rule-viewport" role="rowgroup" tabindex="0" @scroll.passive="onScroll">
      <div class="rule-spacer" :style="{ height: `${items.length * rowHeight}px` }">
        <div class="rule-window" :style="{ transform: `translateY(${start * rowHeight}px)` }">
          <div v-for="entry in visible" :key="`${entry.rule.lineNo}-${entry.rule.type}-${entry.rule.payload}`" class="rule-row" :class="{ 'rule-disabled': entry.rule.disabled }" role="row" :aria-rowindex="entry.index + 2">
            <span class="rule-line" role="cell">{{ t(entry.rule.lineNo) }}</span>
            <span class="rule-payload" role="cell" :title="entry.rule.payload || '-'">{{ entry.rule.payload || '-' }}</span>
            <span role="cell"><span class="rule-type">{{ entry.rule.type }}</span></span>
            <span class="rule-policy-cell" role="cell"><span class="rule-policy" :class="proxyClass(entry.rule.proxy)">{{ entry.rule.proxy }}</span></span>
            <span class="rule-toggle-cell" role="cell">
              <button type="button" class="rule-toggle" role="switch" :aria-checked="!entry.rule.disabled" :aria-label="t(`启用第 ${entry.rule.lineNo} 条规则 ${entry.rule.payload || entry.rule.type}`)" :disabled="busy || !entry.rule.canToggle" :title="t(entry.rule.canToggle ? (entry.rule.disabled ? '已禁用，点击启用' : '已启用，点击禁用') : '当前内核不支持规则开关')" @click="emit('toggle', entry.rule)"><span /></button>
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
