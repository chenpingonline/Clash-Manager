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
</script>

<template>
  <div v-if="modelValue.length" class="selected-members-preview">
    <span v-for="(item, index) in modelValue.slice(0, 2)" :key="index" class="selected-member-preview" :title="item">{{ item }}</span>
    <button type="button" class="rule-text-action" aria-haspopup="dialog" :aria-label="`${title} (${modelValue.length})`" @click="open = true">{{ t('查看全部（{arg0}）', { arg0: modelValue.length }) }}</button>
  </div>
  <SequenceFormDialog :open="open" :title="title" :subtitle="t('已引入 {arg0} 项', { arg0: modelValue.length })" @close="open = false">
    <input v-model="search" class="selected-member-search" :aria-label="t('搜索已引入的成员或集合')" :placeholder="t('搜索已引入的成员或集合')" />
    <ol v-if="filtered.length" class="selected-member-list" role="list">
      <li v-for="item in filtered" :key="item.index" role="listitem"><span class="muted">{{ item.index + 1 }}</span><span>{{ item.name }}</span><button type="button" class="rule-text-action rule-delete-action" :disabled="disabled" :aria-label="t(provider ? '移除集合 {arg0}' : '移除代理 {arg0}', { arg0: item.name })" @click="remove(item.index)">{{ t('移除') }}</button></li>
    </ol>
    <p v-else class="selected-member-empty muted">{{ t(modelValue.length ? '没有匹配的条目' : provider ? '尚未引入集合' : '尚未引入代理') }}</p>
  </SequenceFormDialog>
</template>

<style scoped>
.selected-members-preview{display:flex;align-items:center;gap:5px;min-width:0;height:24px}
.selected-member-preview{flex:1;min-width:0;max-width:140px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;padding:3px 6px;border:1px solid var(--line);border-radius:5px;background:var(--surface-inset);font-size:11px}
.selected-members-preview>button{flex:none;font-size:11px;padding:2px 3px;white-space:nowrap}
.selected-member-search{position:sticky;top:0;z-index:1;background:var(--panel);font-size:12px;margin-bottom:10px}
.selected-member-list{padding:0;margin:0;list-style:none;border:1px solid var(--line);border-radius:9px;overflow:hidden}
.selected-member-list li{display:grid;grid-template-columns:32px minmax(0,1fr) auto;gap:10px;align-items:center;padding:8px 10px;font-size:12px}
.selected-member-list li+li{border-top:1px solid var(--line)}.selected-member-list li>span:nth-child(2){overflow-wrap:anywhere;white-space:pre-wrap;line-height:1.6}
.selected-member-empty{text-align:center;padding:20px;font-size:12px}
</style>
