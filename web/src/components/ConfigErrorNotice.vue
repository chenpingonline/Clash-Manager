<script setup lang="ts">
import { computed } from 'vue'
import { t } from '@/services/i18n'
import { describeConfigError } from '@/services/config-error'

const props = defineProps<{ message: unknown }>()
const diagnostic = computed(() => describeConfigError(props.message))
</script>

<template>
  <div v-if="diagnostic" class="config-error-notice">
    <strong>{{ t('配置校验失败') }}</strong>
    <p v-if="diagnostic.saved" class="config-error-state">{{ t('增强内容已保存，但未能应用。') }}</p>
    <p>{{ t(diagnostic.reason, diagnostic.params) }}</p>
    <p class="config-error-suggestion">{{ t(diagnostic.suggestion, diagnostic.suggestionParams) }}</p>
    <details><summary>{{ t('查看原始错误') }}</summary><pre>{{ diagnostic.raw }}</pre></details>
  </div>
  <span v-else>{{ t(message) }}</span>
</template>

<style scoped>
.config-error-notice{min-width:0;font-size:12px;line-height:1.6;overflow-wrap:anywhere}
.config-error-notice>strong{display:block;color:var(--bad);font-size:13px}
.config-error-notice p{margin:5px 0}
.config-error-state{color:var(--muted)}
.config-error-suggestion{color:var(--text)}
.config-error-notice details{margin-top:6px;color:var(--muted)}
.config-error-notice summary{cursor:pointer;font-size:11px}
.config-error-notice pre{max-height:180px;overflow:auto;margin:6px 0 0;padding:8px;border:1px solid var(--line);border-radius:6px;background:var(--surface-inset);font:11px/1.5 var(--font-mono,monospace);white-space:pre-wrap;overflow-wrap:anywhere}
</style>
