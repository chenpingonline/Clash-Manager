<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import AsyncState from '@/components/AsyncState.vue'
import ConfigEditor from '@/components/ConfigEditor.vue'
import ConfigTextView from '@/components/ConfigTextView.vue'
import { APP_PREFIX, errorMessage, isAbortError } from '@/services/api'
import { notify } from '@/services/toast'
import type { ConfigFormat } from '@/services/config-preview'

interface EffectiveConfig { configPath?: string; path?: string; pid?: number; content?: string; mode?: string }

const CONFIG_MEMORY_TTL = 60_000
let configCache: { value: EffectiveConfig; etag: string; expiresAt: number } | null = null
let evictionTimer = 0

function rememberConfig(value: EffectiveConfig, etag: string) {
  configCache = { value, etag, expiresAt: Date.now() + CONFIG_MEMORY_TTL }
  window.clearTimeout(evictionTimer)
  evictionTimer = window.setTimeout(() => { configCache = null }, CONFIG_MEMORY_TTL)
}

function currentCache() {
  if (configCache && configCache.expiresAt > Date.now()) return configCache
  configCache = null
  return null
}

const loading = ref(true)
const error = ref('')
const config = ref<EffectiveConfig | null>(null)
const query = ref('')
const editing = ref(false)
const format = ref<Exclude<ConfigFormat, 'original'>>('compact')
const activeMatch = ref(0)
const matchCount = ref(0)
const textView = ref<InstanceType<typeof ConfigTextView> | null>(null)
const copying = ref(false)
let request: AbortController | null = null
let alive = true

function moveMatch(offset: number) {
  textView.value?.moveMatch(offset)
}

function searchKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter') return
  event.preventDefault()
  moveMatch(event.shiftKey ? -1 : 1)
}

function searchChanged(state: { count: number; active: number }) {
  matchCount.value = state.count
  activeMatch.value = state.active
}

async function copyConfig() {
  if (copying.value || !config.value?.content) return
  copying.value = true
  const content = config.value.content
  try {
    if (navigator.clipboard && window.isSecureContext) await navigator.clipboard.writeText(content)
    else {
      // NAS pages can use HTTP, where the modern clipboard API is unavailable.
      const field = document.createElement('textarea')
      const focused = document.activeElement
      field.value = content
      field.style.cssText = 'position:fixed;left:-10000px;top:0;opacity:0'
      document.body.appendChild(field)
      try {
        field.select()
        if (!document.execCommand('copy')) throw new Error('浏览器未允许复制配置')
      } finally {
        field.remove()
        if (focused instanceof HTMLElement) focused.focus({ preventScroll: true })
      }
    }
    if (alive) notify('已复制完整配置')
  } catch (cause) { if (alive) notify(errorMessage(cause), true) }
  finally { copying.value = false }
}

async function loadConfig(showLoading = config.value === null) {
  if (!alive) return
  request?.abort()
  const controller = new AbortController()
  request = controller
  const cached = currentCache()
  if (!config.value && cached) config.value = cached.value
  if (showLoading && !config.value) loading.value = true
  try {
    const response = await fetch(`${APP_PREFIX}/api/config/effective`, {
      headers: cached?.etag ? { 'If-None-Match': cached.etag } : undefined,
      signal: controller.signal,
    })
    if (!alive || controller.signal.aborted) return
    if (response.status === 304 && cached) {
      rememberConfig(cached.value, cached.etag)
      error.value = ''
      return
    }
    const payload = await response.json() as EffectiveConfig & { error?: string; message?: string }
    if (!alive || controller.signal.aborted) return
    if (!response.ok) throw new Error(payload.error || payload.message || `HTTP ${response.status}`)
    config.value = payload
    rememberConfig(payload, response.headers.get('etag') || '')
    error.value = ''
  } catch (cause) {
    if (alive && !controller.signal.aborted && !isAbortError(cause) && !config.value) error.value = errorMessage(cause)
  } finally {
    if (alive && request === controller) { loading.value = false; request = null }
  }
}

defineExpose({ refreshPage: () => loadConfig(false) })
async function editorSaved() {
  configCache = null
  await loadConfig(false)
}
onMounted(() => {
  const cached = currentCache()
  if (cached) {
    config.value = cached.value
    loading.value = false
    void loadConfig(false)
  } else {
    void loadConfig(true)
  }
})
onBeforeUnmount(() => { alive = false; request?.abort() })
</script>

<template>
  <Teleport defer to="#page-actions">
    <div class="config-tools">
      <div class="config-search-wrap">
        <input v-model="query" type="search" class="config-search" placeholder="搜索配置" aria-label="搜索配置内容" @keydown="searchKeydown">
        <span v-if="query.trim()" class="config-search-count" role="status">{{ matchCount ? `${activeMatch}/${matchCount}` : '0/0' }}</span>
      </div>
      <button class="ghost config-search-nav" type="button" :disabled="!matchCount" aria-label="上一个匹配项" title="上一个匹配项（Shift + Enter）" @click="moveMatch(-1)">↑</button>
      <button class="ghost config-search-nav" type="button" :disabled="!matchCount" aria-label="下一个匹配项" title="下一个匹配项（Enter）" @click="moveMatch(1)">↓</button>
      <button class="ghost" type="button" @click="format = format === 'formatted' ? 'compact' : 'formatted'">{{ format === 'formatted' ? '压缩' : '格式化' }}</button>
      <button class="ghost" type="button" :disabled="copying || !config?.content" @click="copyConfig">复制配置</button>
      <button v-if="config?.mode === 'managed'" class="ghost" type="button" @click="editing = true">编辑配置</button>
    </div>
  </Teleport>
  <AsyncState :loading="loading" :error="error">
    <div v-if="config" class="config-workspace">
      <div class="config-meta"><span class="tag">当前生效</span><span class="mono config-path" :title="config.path || config.configPath || ''">{{ config.path || config.configPath || '未检测到启动配置路径' }}</span><span v-if="config.pid" class="muted">PID {{ config.pid }}</span><span class="tag config-readonly-tag">{{ config.mode === 'managed' ? '托管配置' : '外部配置 · 只读' }}</span></div>
      <ConfigTextView ref="textView" :content="config.content || ''" :query="query" :format="format" @search="searchChanged" />
    </div>
  </AsyncState>
  <ConfigEditor :open="editing" @close="editing = false" @saved="editorSaved" />
</template>
