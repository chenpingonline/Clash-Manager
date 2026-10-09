<script setup lang="ts">
import { computed } from 'vue'
import { t } from '@/services/i18n'
import { ruleTypes, ruleTypeLabels } from '@/services/profile-sequences'
import PolicySelect from './PolicySelect.vue'

const props = defineProps<{ modelValue: string; disabled: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const options = computed(() => ruleTypes.map(type => ({ type, label: `${t(ruleTypeLabels[type])} (${type})` })))
const selected = computed(() => options.value.find(option => option.type === props.modelValue)?.label || props.modelValue)
function select(label: string) {
  const option = options.value.find(option => option.label === label)
  if (option) emit('update:modelValue', option.type)
}
</script>

<template>
  <PolicySelect class="rule-type-select" :model-value="selected" :options="options.map(option => option.label)" :disabled="disabled" :label="t('规则类型')" :search-placeholder="t('搜索规则说明或类型代码')" panel-class="rule-type-select-panel" @update:model-value="select" />
</template>
