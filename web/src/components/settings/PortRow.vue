<script setup lang="ts">
import { t } from '@/services/i18n'

const props = defineProps<{ modelValue: { enabled?: boolean; port?: number }; id: string; title: string; description: string; locked?: boolean; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: { enabled: boolean; port: number }]; change: [] }>()
function update(partial: Partial<{ enabled: boolean; port: number }>) { emit('update:modelValue', { enabled: props.locked ? true : Boolean(props.modelValue.enabled), port: Number(props.modelValue.port || 0), ...partial }); emit('change') }
</script>
<template><div class="port-row" :data-port-row="id" :class="{ 'mixed-disabled': disabled }"><div class="port-row-copy"><strong :id="`${id}-label`">{{ t(title) }}</strong><span :id="`${id}-description`">{{ t(description) }}</span></div><div class="port-row-control"><input class="port-input mono" :id="id" :aria-labelledby="`${id}-label`" :aria-describedby="disabled ? `${id}-description mixed-port-hint` : `${id}-description`" type="number" min="1" max="65535" :value="modelValue.port || ''" :disabled="disabled" @change="update({ port: Number(($event.target as HTMLInputElement).value) })"><span v-if="locked" class="port-required">{{ t("必需") }}</span><label v-else class="switch"><input type="checkbox" :aria-labelledby="`${id}-label`" :aria-describedby="disabled ? 'mixed-port-hint' : undefined" :checked="modelValue.enabled" :disabled="disabled" @change="update({ enabled: ($event.target as HTMLInputElement).checked })"><span /></label></div></div></template>
