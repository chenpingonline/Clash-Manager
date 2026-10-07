<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import BaseModal from '@/components/BaseModal.vue'
import { api, errorMessage, jsonRequest } from '@/services/api'
import { notify } from '@/services/toast'

const props = defineProps<{ open: boolean }>()
const YamlEditor = defineAsyncComponent(() => import('./YamlEditor.vue'))
const emit = defineEmits<{ close: []; saved: [] }>()
const loading = ref(false), saving = ref(false), error = ref('')
const formatting = ref(false)
const discarding = ref(false)
const closeButton = ref<HTMLButtonElement | null>(null), continueButton = ref<HTMLButtonElement | null>(null), discardButton = ref<HTMLButtonElement | null>(null)
const content = ref(''), original = ref(''), revision = ref(''), path = ref('')
const dirty = computed(() => content.value !== original.value)
let alive = true
let loadVersion = 0

async function load() {
  const version = ++loadVersion
  discarding.value = false
  if (!props.open) { content.value = ''; original.value = ''; revision.value = ''; return }
  loading.value = true
  error.value = ''
  revision.value = ''
  try {
    const result = await api<{ content: string; revision: string; path: string }>('/api/config/editor')
    if (!alive || version !== loadVersion) return
    content.value = original.value = result.content
    revision.value = result.revision
    path.value = result.path
  } catch (cause) {
    if (alive && version === loadVersion) error.value = errorMessage(cause)
  } finally { if (alive && version === loadVersion) loading.value = false }
}

function close() {
  if (saving.value || discarding.value) return
  if (dirty.value) { discarding.value = true; return }
  emit('close')
}

async function cancelDiscard() {
  discarding.value = false
  await nextTick()
  closeButton.value?.focus({ preventScroll: true })
}

function discardChanges() {
  if (saving.value) return
  discarding.value = false
  emit('close')
}

function discardKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault(); event.stopPropagation(); void cancelDiscard()
  } else if (event.key === 'Tab') {
    event.preventDefault()
    const next = event.target === continueButton.value ? discardButton.value : continueButton.value
    next?.focus()
  }
}
watch(discarding, async open => { if (open) { await nextTick(); continueButton.value?.focus({ preventScroll: true }) } })

async function save() {
  if (saving.value || formatting.value || !revision.value || !dirty.value || !content.value.trim()) return
  saving.value = true
  error.value = ''
  try {
    await api('/api/config/editor', jsonRequest('PUT', { content: content.value, revision: revision.value }))
    original.value = content.value
    notify('配置已校验、保存并应用，重启后仍然有效')
    emit('saved')
    emit('close')
  } catch (cause) { error.value = errorMessage(cause) }
  finally { saving.value = false }
}

function beforeUnload(event: BeforeUnloadEvent) {
  if (!props.open || (!dirty.value && !saving.value)) return
  event.preventDefault()
  event.returnValue = ''
}
window.addEventListener('beforeunload', beforeUnload)
onBeforeUnmount(() => { alive = false; loadVersion++; window.removeEventListener('beforeunload', beforeUnload) })
watch(() => props.open, load, { immediate: true })
</script>

<template>
  <BaseModal :open="open" title="编辑托管配置" :show-header="false" card-class="profile-extension-modal config-editor-modal" :closable="!saving && !discarding" :inert="discarding" @close="close">
    <p class="config-editor-note">修改当前启动 YAML。保存时会校验、备份并应用；订阅更新并应用后可能覆盖这些修改，长期修改请使用订阅增强。Controller 地址与 Secret 会保留当前连接设置。</p>
    <div v-if="path && revision" class="config-editor-path mono" :title="path">{{ path }}</div>
    <div v-if="loading" class="profile-extension-loading">正在读取配置…</div>
    <YamlEditor v-else-if="revision" v-model="content" :disabled="saving" @busy="formatting = $event" @error="error = $event" />
    <p v-if="error" class="config-editor-error" role="alert">{{ error }}</p>
    <div class="actions profile-extension-actions config-editor-actions">
      <button class="small" :disabled="loading || saving || formatting || !revision || !dirty || !content.trim()" @click="save">{{ saving ? '正在校验并应用…' : '保存并应用' }}</button>
      <button ref="closeButton" class="ghost small" :disabled="saving" @click="close">{{ dirty ? '放弃修改' : '关闭' }}</button>
    </div>
  </BaseModal>
  <BaseModal :open="open && discarding" title="放弃修改？" card-class="config-discard-modal" @close="cancelDiscard">
    <div @keydown="discardKeydown">
      <p class="muted">尚未保存的配置修改将丢失，是否放弃？</p>
      <div class="actions config-editor-actions">
        <button ref="continueButton" class="ghost small" type="button" @click="cancelDiscard">继续编辑</button>
        <button ref="discardButton" class="danger small" type="button" @click="discardChanges">放弃修改</button>
      </div>
    </div>
  </BaseModal>
</template>
