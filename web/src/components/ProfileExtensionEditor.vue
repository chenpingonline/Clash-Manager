<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import BaseModal from '@/components/BaseModal.vue'
import ProfileSequenceEditor from '@/components/ProfileSequenceEditor.vue'
import RuleSequenceHelp from '@/components/RuleSequenceHelp.vue'
import ProfileExtensionHelp from '@/components/ProfileExtensionHelp.vue'
import { emptySequence, parseSequence, serializeSequence } from '@/services/profile-sequences'
import type { SequenceEditorData, SequenceExtension, SequenceKind } from '@/services/profile-sequences'
import { api, errorMessage, jsonRequest } from '@/services/api'
import { streamProfileJob } from '@/services/profile-jobs'
import { notify } from '@/services/toast'
import type { ProfileExtensionKind, ProfileItem, ProfileJob } from '@/types/api'

const props = withDefaults(defineProps<{ open: boolean; profile: ProfileItem | null; kind: ProfileExtensionKind | null; global?: boolean }>(), { global: false })
const emit = defineEmits<{ close: []; saved: [] }>()
const content = ref(''), loading = ref(false), saving = ref(false), customized = ref(false)
const applyMessage = ref(''), original = ref(''), resetting = ref(false)
const saveButton = ref<HTMLButtonElement | null>(null), cancelButton = ref<HTMLButtonElement | null>(null), resetButton = ref<HTMLButtonElement | null>(null), keepButton = ref<HTMLButtonElement | null>(null), confirmResetButton = ref<HTMLButtonElement | null>(null)
const isRules = computed(() => !props.global && props.kind === 'rules')
const dirty = computed(() => content.value !== original.value)
const hasSequenceChanges = computed(() => { if (!sequenceKind.value) return false; try { const model = parseSequence(content.value, sequenceKind.value); return !!(model.prepend.length || model.append.length || model.delete.length) } catch { return !!content.value.trim() } })
const resetTitle = computed(() => `重置本订阅${sequenceKind.value === 'proxies' ? '节点' : sequenceKind.value === 'groups' ? '代理组' : '规则'}增强？`)
async function cancelReset() { resetting.value = false; await nextTick(); resetButton.value?.focus({ preventScroll: true }) }
async function confirmReset() { resetting.value = false; await reset(); await nextTick(); (saveButton.value?.disabled ? cancelButton.value : saveButton.value)?.focus({ preventScroll: true }) }
function resetKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); void cancelReset() }
  if (event.key === 'Tab') { event.preventDefault(); (event.target === keepButton.value ? confirmResetButton.value : keepButton.value)?.focus() }
}
watch(resetting, async value => { if (value) { await nextTick(); keepButton.value?.focus({ preventScroll: true }) } })
const advanced = ref(false), visualError = ref('')
const sequenceData = ref<SequenceEditorData | null>(null), sequenceModel = ref<SequenceExtension>(emptySequence())
const sequenceKind = computed(() => !props.global && ['rules', 'proxies', 'groups'].includes(props.kind || '') ? props.kind as SequenceKind : null)
let loadGeneration = 0
function updateSequence(value: SequenceExtension) { sequenceModel.value = value; content.value = serializeSequence(value) }
function toggleAdvanced() {
  if (advanced.value && sequenceKind.value) {
    try { sequenceModel.value = parseSequence(content.value, sequenceKind.value); visualError.value = '' }
    catch (cause) { visualError.value = errorMessage(cause); return }
  }
  advanced.value = !advanced.value
}

const metadata: Record<ProfileExtensionKind, { title: string }> = {
  rules: { title: '编辑规则' },
  proxies: { title: '编辑节点' },
  groups: { title: '编辑代理组' },
  override: { title: '扩展覆写配置' },
  script: { title: '扩展脚本' },
}
const meta = computed(() => props.kind ? metadata[props.kind] : metadata.rules)
const endpoint = computed(() => props.kind
  ? props.global
    ? `/api/profiles/global/extensions/${props.kind}`
    : `/api/profiles/${props.profile?.id}/extensions/${props.kind}`
  : '')
const title = computed(() => props.global ? `全局${meta.value.title}` : `${meta.value.title} · ${props.profile?.name || ''}`)

async function load() {
  if (!props.open || !props.kind || (!props.global && !props.profile)) return
  const generation = ++loadGeneration
  loading.value = true
  resetting.value = false
  advanced.value = false; visualError.value = ''; sequenceData.value = null
  try {
    const result = await api<SequenceEditorData>(endpoint.value + (sequenceKind.value ? '/editor' : ''))
    if (generation !== loadGeneration || !props.open) return
    content.value = original.value = result.content || ''
    customized.value = Boolean(result.customized)
    if (sequenceKind.value) {
      sequenceData.value = result
      try { sequenceModel.value = parseSequence(content.value, sequenceKind.value) }
      catch (cause) { advanced.value = true; visualError.value = errorMessage(cause) }
    }
  } catch (cause) {
    if (generation !== loadGeneration || !props.open) return
    notify(errorMessage(cause), true)
    emit('close')
  } finally { if (generation === loadGeneration) loading.value = false }
}

async function save() {
  if (!props.kind || (!props.global && !props.profile)) return
  if (sequenceKind.value) {
    try { parseSequence(content.value, sequenceKind.value) } catch (cause) { notify(errorMessage(cause), true); return }
  }
  saving.value = true
  applyMessage.value = ''
  try {
    let result = await api<ProfileJob & { applied?: boolean }>(endpoint.value, jsonRequest('PUT', { content: content.value, apply: true }))
    if (result.jobId) {
      applyMessage.value = result.message || '准备应用当前配置…'
      result = await streamProfileJob(result.jobId, job => { applyMessage.value = job.message || '正在应用当前配置…' }, new AbortController().signal) as ProfileJob & { applied?: boolean }
      if (result.state === 'failed') throw new Error(`${title.value}已保存，但应用当前配置失败：${result.error ? String(result.error) : '未知错误'}`)
    }
    customized.value = true
    notify(result.jobId ? `${title.value}已保存并应用，配置已立即生效` : `${title.value}已保存；当前没有正在使用的配置`)
    emit('saved')
    emit('close')
  } catch (cause) { notify(errorMessage(cause), true) }
  finally { saving.value = false; applyMessage.value = '' }
}

async function reset() {
  if (!props.kind || (!props.global && !props.profile)) return
  if (sequenceKind.value) {
    updateSequence(emptySequence()); advanced.value = false; visualError.value = ''
    notify('已恢复默认内容，保存并应用后生效')
    return
  }
  saving.value = true
  try {
    await api(endpoint.value, { method: 'DELETE' })
    notify(`${title.value}已恢复默认`)
    emit('saved')
    emit('close')
  } catch (cause) { notify(errorMessage(cause), true) }
  finally { saving.value = false }
}

watch(() => [props.open, props.profile?.id, props.kind, props.global] as const, load, { immediate: true })
</script>

<template>
  <BaseModal :open="open" :title="title" :card-class="`profile-extension-modal ${sequenceKind ? 'profile-sequence-modal' : ''} ${sequenceKind ? 'profile-table-modal' : ''} ${isRules ? 'profile-rules-modal' : ''}`" :closable="!saving && !resetting" :inert="resetting" @close="emit('close')">
    <template #header><div class="sequence-modal-heading"><div class="sequence-modal-title"><h3>{{ title }}</h3><RuleSequenceHelp v-if="kind === 'rules' && !global" /><ProfileExtensionHelp v-else-if="kind" :kind="kind" /></div><div v-if="sequenceKind" class="sequence-modal-actions"><button class="ghost small" :disabled="loading || saving" @click="toggleAdvanced">{{ advanced ? '可视化' : '高级' }}</button></div></div></template>
    <div v-if="loading" class="profile-extension-loading">正在读取…</div>
    <ProfileSequenceEditor v-else-if="sequenceKind && sequenceData && !advanced" :kind="sequenceKind" :model-value="sequenceModel" :data="sequenceData" :disabled="saving" @update:model-value="updateSequence" @advanced="advanced = true" />
    <template v-else><p v-if="visualError" class="sequence-error" role="alert">{{ visualError }}</p><textarea v-model="content" class="editor profile-extension-editor" spellcheck="false" :aria-label="meta.title" :disabled="saving" /></template>
    <div class="actions profile-extension-actions" :class="{ 'rule-editor-footer': !!sequenceKind }">
      <span v-if="sequenceKind" class="muted rule-footer-note">修改仅作用于当前订阅，更新订阅后保留。</span>
      <button ref="saveButton" class="small" :disabled="loading || saving || (!!sequenceKind && !dirty)" @click="save">{{ saving ? (applyMessage || '保存并应用中…') : '保存并应用' }}</button>
      <button v-if="sequenceKind || customized" ref="resetButton" class="danger small" :disabled="loading || saving || (!!sequenceKind && !hasSequenceChanges)" @click="sequenceKind ? resetting = true : reset()">{{ sequenceKind ? '重置本订阅增强' : '恢复默认' }}</button>
      <button ref="cancelButton" class="ghost small" :disabled="saving" @click="emit('close')">取消</button>
    </div>
  </BaseModal>
  <BaseModal :open="open && resetting" :title="resetTitle" card-class="config-discard-modal" @close="cancelReset">
    <div @keydown="resetKeydown"><p class="muted">将清除此编辑器中的前置、后置条目与排除记录。保存并应用后生效；其他增强保持不变。</p><div class="actions config-editor-actions"><button ref="keepButton" type="button" class="ghost small" @click="cancelReset">继续编辑</button><button ref="confirmResetButton" type="button" class="danger small" @click="confirmReset">重置当前增强</button></div></div>
  </BaseModal>
</template>

<style scoped>
.sequence-modal-title{position:relative;display:flex;align-items:center;gap:8px;min-width:0}
.sequence-modal-title h3{overflow-wrap:anywhere}
.sequence-modal-actions{display:flex;align-items:center;gap:8px;flex:none}
</style>
