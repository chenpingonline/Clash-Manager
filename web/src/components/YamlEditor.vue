<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Compartment, EditorState, Transaction } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab, isolateHistory } from '@codemirror/commands'
import { yamlViewExtensions } from '@/services/yaml-code-view'
import type { ConfigFormat } from '@/services/config-preview'
import type { ConfigFormatRequest, ConfigFormatResponse } from '@/services/config-format.worker'

const props = defineProps<{ modelValue: string; disabled: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [content: string]; busy: [value: boolean]; error: [message: string] }>()
const host = ref<HTMLElement | null>(null), formatting = ref(false)
const locked = new Compartment()
let view: EditorView | null = null, worker: Worker | null = null, requestId = 0

function lock() {
  view?.dispatch({ effects: locked.reconfigure([
    EditorState.readOnly.of(props.disabled || formatting.value), EditorView.editable.of(!props.disabled && !formatting.value),
  ]) })
}
function busy(value: boolean) { formatting.value = value; emit('busy', value); lock() }
function format(format: Exclude<ConfigFormat, 'original'>) {
  if (!view || !worker || props.disabled || formatting.value) return
  emit('error', '')
  busy(true)
  worker.postMessage({ id: ++requestId, content: view.state.doc.toString(), format } satisfies ConfigFormatRequest)
}

onMounted(() => {
  if (!host.value) return
  view = new EditorView({ parent: host.value, state: EditorState.create({ doc: props.modelValue, extensions: [
    ...yamlViewExtensions(), history(), keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
    EditorView.contentAttributes.of({ 'aria-label': '托管配置 YAML', spellcheck: 'false' }),
    locked.of([EditorState.readOnly.of(props.disabled), EditorView.editable.of(!props.disabled)]),
    EditorView.updateListener.of(update => { if (update.docChanged) emit('update:modelValue', update.state.doc.toString()) }),
  ] }) })
  try {
    worker = new Worker(new URL('../services/config-format.worker.ts', import.meta.url), { type: 'module' })
    worker.onmessage = (event: MessageEvent<ConfigFormatResponse>) => {
      if (!view || event.data.id !== requestId) return
      busy(false)
      if (event.data.error) { emit('error', event.data.error); return }
      const content = event.data.content
      if (content !== undefined && content !== view.state.doc.toString()) view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: content }, selection: { anchor: 0 },
        effects: EditorView.scrollIntoView(0, { y: 'start' }), annotations: [Transaction.userEvent.of('input.format'), isolateHistory.of('full')],
      })
    }
    worker.onerror = () => { busy(false); worker?.terminate(); worker = null; emit('error', '格式转换不可用，请重新打开编辑器') }
  } catch { emit('error', '此浏览器无法运行配置格式转换') }
})
watch(() => props.disabled, lock)
watch(() => props.modelValue, content => {
  if (view && content !== view.state.doc.toString()) {
    requestId++; busy(false)
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: content } })
  }
})
onBeforeUnmount(() => { requestId++; worker?.terminate(); view?.destroy(); view = null; emit('busy', false) })
</script>

<template>
  <div class="yaml-editor-tools">
    <button class="ghost small" type="button" :disabled="disabled || formatting" @click="format('formatted')">格式化</button>
    <button class="ghost small" type="button" :disabled="disabled || formatting" @click="format('compact')">压缩</button>
    <span v-if="formatting" class="muted" role="status">正在调整格式…</span>
    <span v-else class="muted">格式调整可撤销，保存后生效</span>
  </div>
  <div ref="host" class="yaml-code-editor" />
</template>
