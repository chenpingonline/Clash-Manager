<script setup lang="ts">
import { computed } from 'vue'
import HelpPopover from '@/components/HelpPopover.vue'
import { extensionExamples } from '@/services/profile-extension-examples'
import type { ProfileExtensionKind } from '@/types/api'
const props = defineProps<{ kind: ProfileExtensionKind }>()
const example = computed(() => extensionExamples[props.kind])
</script>

<template>
  <HelpPopover class="profile-extension-help" :label="example.title">
    <strong>{{ example.title }} · {{ example.language }}</strong>
    <span>{{ example.description }}</span>
    <pre class="extension-example-code"><code>{{ example.content }}</code></pre>
    <span v-for="note in example.notes" :key="note">{{ note }}</span>
    <span v-if="kind === 'override' || kind === 'script'">顺序：全局覆写 → 全局脚本 → 对应订阅覆写 → 对应订阅脚本。</span>
  </HelpPopover>
</template>

<style scoped>
.profile-extension-help{position:static;flex:none}
.profile-extension-help :deep(.help-popover-panel){left:0;right:auto;width:min(520px,calc(100vw - 72px));max-height:calc(100dvh - 160px);overflow:auto;scrollbar-width:auto;scrollbar-color:auto}
.profile-extension-help :deep(.help-popover-panel::before){display:none}
.extension-example-code{margin:10px 0;padding:10px;border:1px solid var(--line);border-radius:7px;background:var(--surface-inset);color:var(--text);font-family:"SFMono-Regular",Consolas,monospace;font-size:11px;line-height:1.6;white-space:pre;overflow:auto;scrollbar-width:auto;scrollbar-color:auto;user-select:text}
</style>
