<script setup lang="ts">
import { computed } from 'vue'
import AppSelect from '@/components/AppSelect.vue'
import { t } from '@/services/i18n'
import type { ProxyNodeSort } from '@/services/proxy-view'

defineProps<{ modelValue: ProxyNodeSort; label: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: ProxyNodeSort] }>()
const options = computed(() => [
  { value: 'default', label: t('默认排序') },
  { value: 'delay', label: t('按延迟') },
  { value: 'name', label: t('按名称') },
])
</script>

<template>
  <AppSelect class="proxy-sort-trigger" :model-value="modelValue" :aria-label="label" :options="options" @update:model-value="emit('update:modelValue', $event)" />
</template>

<style scoped>
.proxy-sort-trigger.app-select { width: 90px; min-width: 90px; min-height: 30px; height: 30px; gap: 2px; padding: 0 4px; border-radius: 8px; background: transparent; font-size: 11px; }
.proxy-sort-trigger :deep(> svg) { width: 12px; height: 12px; }
</style>
