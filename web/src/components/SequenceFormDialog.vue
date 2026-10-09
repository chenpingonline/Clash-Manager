<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { t } from '@/services/i18n'

const props = defineProps<{ open: boolean; title: string; subtitle?: string; inlineSubtitle?: boolean; hideFooter?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null), body = ref<HTMLElement | null>(null)
watch(() => props.open, async open => {
  await nextTick()
  if (open && props.open) {
    if (!dialog.value?.open) dialog.value?.showModal()
    if (body.value) body.value.scrollTop = 0
  } else if (!props.open) dialog.value?.close()
})
function keyboard(event: KeyboardEvent) {
  if (event.key !== 'Tab') return
  const controls = Array.from(dialog.value?.querySelectorAll<HTMLElement>('button, input, select, textarea, [tabindex="0"]') || [])
    .filter(element => !element.matches(':disabled') && element.getClientRects().length)
  if (!controls.length) return
  event.preventDefault()
  const index = controls.indexOf(document.activeElement as HTMLElement)
  controls[(index + (event.shiftKey ? -1 : 1) + controls.length) % controls.length]?.focus()
}
function backdrop(event: MouseEvent) {
  if (event.target !== dialog.value || !dialog.value) return
  const bounds = dialog.value.getBoundingClientRect()
  if (event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom) emit('close')
}
onBeforeUnmount(() => dialog.value?.close())
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="modal-card sequence-form-dialog" :aria-label="title" @cancel.prevent.stop="emit('close')" @keydown.esc.stop @keydown="keyboard" @click="backdrop">
      <template v-if="open">
        <header class="sequence-dialog-header"><div :class="{ 'sequence-dialog-title-inline': inlineSubtitle }"><h3>{{ title }}</h3><p v-if="subtitle">{{ subtitle }}</p></div><button type="button" class="ghost small" autofocus :aria-label="t('关闭')" @click="emit('close')">×</button></header>
        <div ref="body" class="sequence-dialog-body" tabindex="0" :aria-label="title"><slot /></div>
        <footer v-if="!hideFooter" class="sequence-dialog-footer"><span class="muted">{{ t('修改保留在当前表单，添加或更新代理组后进入草稿。') }}</span><button type="button" class="small" @click="emit('close')">{{ t('返回编辑') }}</button></footer>
      </template>
    </dialog>
  </Teleport>
</template>

<style scoped>
.sequence-form-dialog{width:min(720px,calc(100% - 32px));max-height:calc(100dvh - 32px);margin:auto;color:var(--text)}
.sequence-form-dialog::backdrop{background:rgba(0,0,0,.45);backdrop-filter:blur(2px)}
.sequence-dialog-header{display:flex;align-items:flex-start;justify-content:space-between;gap:16px;margin-bottom:16px}
.sequence-dialog-header>div{min-width:0}.sequence-dialog-header h3{font-size:17px;margin:0}.sequence-dialog-header p{margin:6px 0 0;font-size:12px;color:var(--muted);overflow-wrap:anywhere}
.sequence-dialog-header .sequence-dialog-title-inline{display:flex;align-items:center;gap:10px}
.sequence-dialog-header .sequence-dialog-title-inline p{margin:0;white-space:nowrap}
.sequence-dialog-header button{flex:none;width:30px;height:30px;padding:0;font-size:20px}
.sequence-dialog-body{max-height:min(60dvh,520px);overflow:auto;overscroll-behavior:contain;padding:2px}
.sequence-dialog-footer{display:flex;align-items:center;justify-content:space-between;gap:16px;border-top:1px solid var(--line);padding-top:14px;margin-top:16px}
.sequence-dialog-footer span{font-size:11px;line-height:1.5}.sequence-dialog-footer button{flex:none}
.sequence-dialog-body:focus-visible,.sequence-dialog-header button:focus-visible{outline:2px solid var(--accent);outline-offset:1px}
@media(max-width:480px){.sequence-form-dialog{padding:16px}.sequence-dialog-footer{align-items:flex-end;gap:10px}}
</style>
