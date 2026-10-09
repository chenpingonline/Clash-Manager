<script setup lang="ts">
import { t } from '@/services/i18n'

defineProps<{ modelValue: boolean; title: string; description: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; change: [] }>()
</script>
<template><label class="setting-toggle" :class="{ disabled }"><div><strong>{{ t(title) }}</strong><span>{{ t(description) }}</span></div><span class="switch"><input type="checkbox" :checked="modelValue" :disabled="disabled" @change="emit('update:modelValue', ($event.target as HTMLInputElement).checked); emit('change')"><span /></span></label></template>
