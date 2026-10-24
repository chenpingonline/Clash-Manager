<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import HelpPopover from '@/components/HelpPopover.vue'
import SequenceFormDialog from '@/components/SequenceFormDialog.vue'
import { api, errorMessage, isAbortError } from '@/services/api'
import { t } from '@/services/i18n'
import type { InstalledApp } from '@/services/installed-apps'

const props = defineProps<{ open: boolean; existingRules: string[] }>()
const emit = defineEmits<{ close: []; selected: [apps: InstalledApp[]] }>()
const items = ref<InstalledApp[]>([]), selected = ref<string[]>([]), search = ref(''), loading = ref(false), error = ref('')
let controller: AbortController | null = null
const visible = computed(() => items.value.filter(app => `${app.name} ${app.id}`.toLowerCase().includes(search.value.toLowerCase().trim())))
const existing = computed(() => new Set(props.existingRules))
const chosen = computed(() => items.value.filter(app => selected.value.includes(app.id) && app.rule && !existing.value.has(app.rule)))
async function load() {
  controller?.abort()
  const request = new AbortController()
  controller = request
  loading.value = true; error.value = ''; selected.value = []; items.value = []
  try {
    const result = await api<{ items: InstalledApp[] }>('/api/system/installed-apps', { signal: request.signal })
    if (!request.signal.aborted) items.value = result.items || []
  } catch (cause) { if (!request.signal.aborted && !isAbortError(cause)) error.value = errorMessage(cause) }
  finally { if (controller === request) loading.value = false }
}
watch(() => props.open, open => { if (open) { search.value = ''; void load() } else controller?.abort() })
onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <SequenceFormDialog :open="open" :title="t('选择飞牛应用直连')" compact @close="emit('close')">
    <div class="fnos-app-note"><span>{{ t('所选应用将添加为全局前置直连规则。') }}</span><HelpPopover :teleport="false" :label="t('选择飞牛应用直连')"><span>{{ t('勾选后添加全局前置直连规则，并启用进程识别；点击主编辑器的保存并应用后生效。') }}</span><span>{{ t('仅匹配应用安装目录内的程序，在规则模式下生效；系统解释器、外部子进程和 Docker 容器不保证覆盖。') }}</span></HelpPopover></div>
    <input v-model="search" type="search" :aria-label="t('搜索已安装应用')" :placeholder="t('搜索已安装应用')" />
    <p v-if="loading" role="status">{{ t('正在读取已安装应用…') }}</p>
    <div v-else-if="error" role="alert"><p class="sequence-error">{{ t(error) }}</p><button type="button" class="ghost small" @click="load">{{ t('重试') }}</button></div>
    <p v-else-if="!visible.length" class="muted">{{ t('没有可显示的已安装应用') }}</p>
    <div v-else class="fnos-app-list">
      <div v-for="app in visible" :key="app.id" class="fnos-app-row">
        <label :title="`${app.name} · ${app.id}`"><input v-model="selected" type="checkbox" :value="app.id" :disabled="!app.rule || existing.has(app.rule)" :aria-label="app.name" /><span class="fnos-app-name">{{ app.name }}</span><small>{{ app.id }}</small></label>
        <HelpPopover :teleport="false" v-if="app.reason" :label="t('无法识别')" :trigger-text="t('无法识别')"><span>{{ t(app.reason) }}</span></HelpPopover>
        <span v-else-if="existing.has(app.rule)" class="tag">{{ t('已添加') }}</span>
      </div>
    </div>
    <template #footer><button type="button" class="small" :disabled="loading || !chosen.length" @click="emit('selected', chosen)">{{ t('添加到全局前置规则') }} ({{ chosen.length }})</button><button type="button" class="ghost small" @click="emit('close')">{{ t('取消') }}</button></template>
  </SequenceFormDialog>
</template>

<style scoped>
.fnos-app-note{display:flex;align-items:center;gap:6px;margin-bottom:10px;color:var(--muted);font-size:12px;line-height:1.5}
p{margin:0 0 10px;line-height:1.5}input[type=search]{width:100%;margin-bottom:10px}.fnos-app-list{border:1px solid var(--line);border-radius:7px;overflow:hidden}.fnos-app-row{display:flex;align-items:center;gap:8px;min-height:38px;padding:6px 10px}.fnos-app-row+.fnos-app-row{border-top:1px solid var(--line)}.fnos-app-row>label{display:flex;align-items:center;gap:9px;flex:1;min-width:0;margin:0;cursor:pointer}.fnos-app-row input{flex:none;width:14px;height:14px;min-height:14px;padding:0;margin:0}.fnos-app-name{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:13px;font-weight:500}.fnos-app-row small{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--muted);font-size:11px}.fnos-app-row .tag{flex:none;font-size:11px;padding:2px 7px}.fnos-app-row :deep(.help-popover-trigger){font-size:11px;white-space:nowrap}
@media(max-width:480px){.fnos-app-row small{display:none}.fnos-app-note{align-items:flex-start}}
</style>
