<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { t } from '@/services/i18n'
import SequenceFormDialog from './SequenceFormDialog.vue'

const props = defineProps<{ modelValue: string[]; disabled: boolean; provider?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()
const open = ref(false), search = ref('')
const title = computed(() => t(props.provider ? '管理代理集合' : '管理代理成员'))
const filtered = computed(() => props.modelValue.map((name, index) => ({ name, index })).filter(item => item.name.toLocaleLowerCase().includes(search.value.trim().toLocaleLowerCase())))
watch(open, () => { search.value = '' })
function remove(index: number) {
  if (!props.disabled) emit('update:modelValue', props.modelValue.filter((_, itemIndex) => itemIndex !== index))
}
function removeAll() {
  if (props.disabled || !props.modelValue.length) return
  emit('update:modelValue', [])
  search.value = ''
}
</script>

<template>
  <button v-if="modelValue.length" type="button" class="selected-member-count" aria-haspopup="dialog" :aria-label="`${title} (${modelValue.length})`" @click="open = true">{{ t(provider ? '管理集合（{arg0}）' : '管理成员（{arg0}）', { arg0: modelValue.length }) }}<span aria-hidden="true">›</span></button>
  <SequenceFormDialog :open="open" :title="title" :subtitle="t('已引入 {arg0} 项', { arg0: modelValue.length })" inline-subtitle hide-footer @close="open = false">
    <div class="selected-member-toolbar">
      <input v-model="search" class="selected-member-search" :aria-label="t('搜索已引入的成员或集合')" :placeholder="t('搜索已引入的成员或集合')" />
      <button type="button" class="rule-text-action rule-delete-action selected-member-clear" :disabled="disabled || !modelValue.length" :title="t(provider ? '移除全部已引入的集合' : '移除全部已引入的代理成员')" @click="removeAll">{{ t('全部移除') }}</button>
    </div>
    <ol v-if="filtered.length" class="selected-member-list" role="list">
      <li v-for="item in filtered" :key="item.index" role="listitem"><span class="muted">{{ item.index + 1 }}</span><span>{{ item.name }}</span><button type="button" class="rule-text-action rule-delete-action" :disabled="disabled" :aria-label="t(provider ? '移除集合 {arg0}' : '移除代理 {arg0}', { arg0: item.name })" @click="remove(item.index)">{{ t('移除') }}</button></li>
    </ol>
    <p v-else class="selected-member-empty muted">{{ t(modelValue.length ? '没有匹配的条目' : provider ? '尚未引入集合' : '尚未引入代理') }}</p>
  </SequenceFormDialog>
</template>

<style scoped>
.selected-member-count{flex:none;display:inline-flex;align-items:center;justify-content:center;gap:4px;height:20px;padding:0 6px;border:1px solid color-mix(in srgb,var(--accent) 30%,var(--line));border-radius:6px;background:color-mix(in srgb,var(--accent) 7%,var(--panel));color:var(--accent);font-size:11px;font-weight:600;line-height:18px;white-space:nowrap}
.selected-member-count:hover{background:color-mix(in srgb,var(--accent) 15%,var(--panel))}
.selected-member-count:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
.selected-member-toolbar{position:sticky;top:0;z-index:1;display:flex;align-items:center;gap:10px;background:var(--panel);margin-bottom:10px}
.selected-member-search{flex:1;min-width:0;font-size:12px}
.selected-member-clear{flex:none;white-space:nowrap}
.selected-member-list{padding:0;margin:0;list-style:none;border:1px solid var(--line);border-radius:9px;overflow:hidden}
.selected-member-list li{display:grid;grid-template-columns:32px minmax(0,1fr) auto;gap:10px;align-items:center;padding:8px 10px;font-size:12px}
.selected-member-list li+li{border-top:1px solid var(--line)}.selected-member-list li>span:nth-child(2){overflow-wrap:anywhere;white-space:pre-wrap;line-height:1.6}
.selected-member-empty{text-align:center;padding:20px;font-size:12px}
</style>
