<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Search, X } from 'lucide-vue-next'
import { t } from '@/services/i18n'
import { searchSettings, type SettingsSearchEntry } from '@/services/settings-search'

const props = defineProps<{ entries: SettingsSearchEntry[]; disabled?: boolean }>()
const emit = defineEmits<{ select: [entry: SettingsSearchEntry] }>()
const query = ref(''), expanded = ref(false), active = ref(0)
const root = ref<HTMLElement>(), input = ref<HTMLInputElement>()
const results = computed(() => searchSettings(props.entries, query.value, t))
const showResults = computed(() => expanded.value && Boolean(query.value.trim()))
watch(results, () => { active.value = 0 })
function close() { expanded.value = false }
function outside(event: PointerEvent) { if (!root.value?.contains(event.target as Node)) close() }
function blur(event: FocusEvent) { if (!root.value?.contains(event.relatedTarget as Node | null)) close() }
function clear() { query.value = ''; close(); input.value?.focus() }
function select(entry: SettingsSearchEntry) { query.value = ''; close(); emit('select', entry) }
async function keydown(event: KeyboardEvent) {
  if (event.isComposing) return
  if (event.key === 'Escape') { event.preventDefault(); close(); return }
  if (event.key === 'Enter' && showResults.value && results.value[active.value]) {
    event.preventDefault(); select(results.value[active.value]!); return
  }
  if (!['ArrowDown', 'ArrowUp'].includes(event.key) || !results.value.length) return
  event.preventDefault()
  if (!showResults.value) { expanded.value = true; active.value = 0 }
  else active.value = (active.value + (event.key === 'ArrowDown' ? 1 : -1) + results.value.length) % results.value.length
  await nextTick()
  document.getElementById(`setting-search-result-${active.value}`)?.scrollIntoView({ block: 'nearest' })
}
onMounted(() => document.addEventListener('pointerdown', outside))
onBeforeUnmount(() => document.removeEventListener('pointerdown', outside))
</script>

<template>
  <div ref="root" class="settings-search" @focusout="blur">
    <div class="settings-search-control">
      <Search :size="15" aria-hidden="true" />
      <input ref="input" v-model="query" role="combobox" :aria-label="t('搜索设置')" :placeholder="t('搜索设置…')" :disabled="disabled" autocomplete="off" aria-autocomplete="list" :aria-expanded="showResults" aria-controls="setting-search-results" :aria-activedescendant="showResults && results.length ? `setting-search-result-${active}` : undefined" @input="expanded = true" @focus="expanded = true" @keydown="keydown">
      <button v-if="query" type="button" class="settings-search-clear" :aria-label="t('清空搜索')" @click="clear"><X :size="13" aria-hidden="true" /></button>
    </div>
    <div v-if="showResults" class="settings-search-popover">
      <p class="settings-search-summary" role="status">{{ results.length ? t('找到 {arg0} 项设置', { arg0: results.length }) : t('未找到相关设置') }}</p>
      <div id="setting-search-results" role="listbox" :aria-label="t('搜索结果')" class="settings-search-results">
        <button v-for="(entry, index) in results" :id="`setting-search-result-${index}`" :key="`${entry.section}-${entry.selector}`" type="button" role="option" :aria-selected="active === index" tabindex="-1" @pointerdown.prevent @click="select(entry)" @pointermove="active = index">
          <span>{{ t(entry.label) }}</span><small>{{ entry.path }}</small>
        </button>
      </div>
      <p v-if="!results.length" class="settings-search-empty">{{ t('试试其他名称或参数名，如 DNS、端口、MTU') }}</p>
    </div>
  </div>
</template>

<style scoped>
.settings-search { position: relative; width: clamp(170px, 24vw, 320px); max-width: 100%; }
.settings-search-control { display: flex; align-items: center; gap: 7px; height: 34px; padding: 0 9px; border: 1px solid var(--line); border-radius: 8px; background: var(--control-bg); color: var(--muted); }
.settings-search-control:focus-within { border-color: var(--accent); }
.settings-search-control > svg { flex: none; }
.settings-search-control input { flex: 1; min-width: 0; width: 100%; min-height: 0; height: 30px; padding: 0; border: 0; border-radius: 0; background: transparent; font-size: 13px; outline: none; box-shadow: none; }
.settings-search-control input:focus, .settings-search-control input:focus-visible { outline: none; box-shadow: none; }
.settings-search-control .settings-search-clear { display: grid; place-items: center; width: 22px; height: 22px; min-height: 22px; padding: 0; border: 0; background: transparent; color: var(--muted); }
.settings-search-popover { position: absolute; z-index: 50; top: calc(100% + 8px); left: 0; width: min(420px, calc(100vw - 48px)); padding: 6px; border: 1px solid var(--line); border-radius: 10px; background: var(--panel); box-shadow: var(--shadow-pop); }
.settings-search-summary { margin: 4px 8px 7px; color: var(--muted); font-size: 12px; }
.settings-search-results { max-height: min(360px, 55vh); overflow-y: auto; }
.settings-search-results > button { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; width: 100%; padding: 9px 10px; border: 0; border-radius: 6px; background: transparent; color: var(--text); text-align: left; font-size: 13px; font-weight: 500; white-space: normal; }
.settings-search-results > button[aria-selected="true"] { background: var(--accent-soft); color: var(--accent-ink); }
.settings-search-results small { color: var(--muted); font-size: 11px; font-weight: 400; }
.settings-search-empty { margin: 8px; font-size: 12px; color: var(--muted); white-space: normal; line-height: 1.6; }
@media (max-width: 680px) { .settings-search { width: 100%; } .settings-search-popover { width: min(420px, calc(100vw - 28px)); } }
</style>
