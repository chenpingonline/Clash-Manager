<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import type { EditorView } from '@codemirror/view'
import { configMatchRange, configSearchIndex, configTextDocument, type ConfigSearchIndex, type ConfigTextDocument } from '@/services/config-text'
import type { ConfigTextRequest, ConfigTextResponse } from '@/services/config-text.worker'
import type { ConfigFormat } from '@/services/config-preview'

const props = defineProps<{ content: string; query: string; format: ConfigFormat }>()
const emit = defineEmits<{ search: [state: { count: number; active: number }] }>()
const host = ref<HTMLElement | null>(null), ready = ref(false), activeMatch = ref(0)
const document = shallowRef<ConfigTextDocument | null>(null)
const search = shallowRef<ConfigSearchIndex>({ prefix: [0], count: 0 })
let alive = true, documentId = 0, searchId = 0, renderId = 0
let worker: Worker | null = null, viewer: EditorView | null = null
let engine: typeof import('@/services/yaml-code-view') | null = null
let currentRange: { from: number; to: number } | null = null

async function renderDocument() {
  const value = document.value, id = ++renderId
  if (!value || !host.value) return
  const module = await import('@/services/yaml-code-view')
  if (!alive || id !== renderId || !host.value || value !== document.value) return
  engine = module
  const content = value.lines.join('\n')
  if (viewer) viewer.setState(module.previewState(content, props.query))
  else viewer = module.createConfigViewer(host.value, content, props.query)
  viewer.scrollDOM.classList.add('persistent-horizontal-scrollbar')
  viewer.scrollDOM.scrollTop = 0; viewer.scrollDOM.scrollLeft = 0
  ready.value = true
  if (currentRange) module.revealConfigMatch(viewer, currentRange)
}

function locateMatch() {
  currentRange = null
  if (viewer && engine) engine.revealConfigMatch(viewer, null)
  if (!document.value || !activeMatch.value) return
  if (worker) worker.postMessage({ kind: 'locate', documentId, searchId, query: props.query, match: activeMatch.value } satisfies ConfigTextRequest)
  else {
    currentRange = configMatchRange(document.value, search.value, activeMatch.value, props.query)
    if (viewer && engine && ready.value) engine.revealConfigMatch(viewer, currentRange)
  }
}

function updateSearch(index: ConfigSearchIndex) {
  search.value = index
  activeMatch.value = index.count ? 1 : 0
  locateMatch()
}

function fallback() {
  worker?.terminate(); worker = null
  document.value = configTextDocument(props.content)
  void renderDocument()
  updateSearch(configSearchIndex(document.value.lines, props.query))
}

try {
  worker = new Worker(new URL('../services/config-text.worker.ts', import.meta.url), { type: 'module' })
  worker.onmessage = (event: MessageEvent<ConfigTextResponse>) => {
    const result = event.data
    if (!alive || result.documentId !== documentId) return
    if (result.document) { document.value = result.document; void renderDocument() }
    if (result.searchId !== searchId) return
    if (result.search) updateSearch(result.search)
    if (result.match === activeMatch.value && result.range !== undefined) {
      currentRange = result.range
      if (viewer && engine && ready.value) engine.revealConfigMatch(viewer, currentRange)
    }
  }
  worker.onerror = () => { if (alive) fallback() }
} catch { /* Read-only virtual rendering is still available without a worker. */ }

function prepareDocument() {
  documentId++; searchId++; renderId++
  document.value = null; ready.value = false; currentRange = null
  updateSearch({ prefix: [0], count: 0 })
  if (worker) worker.postMessage({ kind: 'document', documentId, searchId, content: props.content, query: props.query, format: props.format } satisfies ConfigTextRequest)
  else fallback()
}
watch(() => [props.content, props.format], prepareDocument, { immediate: true })
watch(() => props.query, query => {
  searchId++
  updateSearch({ prefix: [0], count: 0 })
  if (viewer && engine) engine.setConfigQuery(viewer, query)
  if (worker) worker.postMessage({ kind: 'search', documentId, searchId, query } satisfies ConfigTextRequest)
  else if (document.value) updateSearch(configSearchIndex(document.value.lines, query))
})
function moveMatch(offset: number) {
  if (!search.value.count) return
  activeMatch.value = ((activeMatch.value - 1 + offset + search.value.count) % search.value.count) + 1
  locateMatch()
}
defineExpose({ moveMatch })
watch([search, activeMatch], ([index, active]) => emit('search', { count: index.count, active }), { immediate: true })
onMounted(() => { if (document.value) void renderDocument() })
onBeforeUnmount(() => { alive = false; renderId++; worker?.terminate(); viewer?.destroy(); viewer = null })
</script>

<template>
  <div class="config-viewer">
    <div ref="host" v-show="ready" class="yaml-code-editor config-code-preview" />
    <div v-if="!ready" class="empty">正在准备配置预览…</div>
  </div>
</template>
