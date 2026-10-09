<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { t } from '@/services/i18n'
import { groupMemberEntries } from '@/services/profile-sequences'
import type { NamedEntry } from '@/services/profile-sequences'

const props = defineProps<{ group: NamedEntry | null }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const closeButton = ref<HTMLButtonElement | null>(null)
const body = ref<HTMLElement | null>(null)
const entries = computed(() => props.group ? groupMemberEntries(props.group) : [])
watch(() => props.group, async group => {
  await nextTick()
  if (group && props.group === group) {
    if (!dialog.value?.open) dialog.value?.showModal()
    if (body.value) body.value.scrollTop = 0
  } else if (!props.group) dialog.value?.close()
})
function cycleFocus() {
  (document.activeElement === closeButton.value ? body.value : closeButton.value)?.focus()
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
    <dialog ref="dialog" class="modal-card group-members-modal" :aria-label="t('成员 / 集合')" @cancel.prevent.stop="emit('close')" @keydown.esc.stop @keydown.tab.prevent="cycleFocus" @click="backdrop">
      <header class="group-members-heading">
        <div><h3>{{ t('成员 / 集合') }}</h3><p>{{ group?.name }}</p></div>
        <button ref="closeButton" type="button" class="ghost small" autofocus @click="emit('close')">{{ t('关闭') }}</button>
      </header>
      <p class="group-members-count muted">{{ t('共 {arg0} 项', { arg0: entries.length }) }}</p>
      <div ref="body" class="group-members-body" tabindex="0" :aria-label="t('成员 / 集合')">
        <ol v-if="entries.length"><li v-for="(entry, index) in entries" :key="index">{{ entry }}</li></ol>
        <p v-else class="muted">{{ t('未配置成员') }}</p>
      </div>
    </dialog>
  </Teleport>
</template>

<style scoped>
.group-members-modal { width: min(620px, calc(100% - 32px)); max-height: calc(100dvh - 32px); margin: auto; color: var(--text); }
.group-members-modal::backdrop { background: rgba(0, 0, 0, .6); backdrop-filter: blur(4px); }
.group-members-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.group-members-heading div { min-width: 0; }
.group-members-heading h3 { margin: 0; font-size: 17px; }
.group-members-heading p { margin: 8px 0 0; overflow-wrap: anywhere; font-size: 13px; }
.group-members-heading button { flex: none; }
.group-members-count { font-size: 12px; margin: 16px 0 8px; }
.group-members-body { max-height: min(60dvh, 480px); overflow: auto; overscroll-behavior: contain; border: 1px solid var(--line); border-radius: 9px; padding: 10px 16px; font-size: 13px; }
.group-members-body ol { margin: 0; padding-left: 30px; }
.group-members-body li { padding: 6px 0 6px 8px; line-height: 1.6; overflow-wrap: anywhere; white-space: pre-wrap; }
.group-members-body li + li { border-top: 1px solid var(--line); }
.group-members-body li::marker { color: var(--muted); }
.group-members-body:focus-visible, .group-members-heading button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
</style>
