<script setup lang="ts">
import { Languages } from 'lucide-vue-next'
import AppSelect from '@/components/AppSelect.vue'
import { languagePreference, setLanguage, t, type LanguagePreference } from '@/services/i18n'
function change(value: LanguagePreference) { setLanguage(value) }
</script>
<template>
  <label class="language-select">
    <Languages :size="15" aria-hidden="true" />
    <AppSelect class="language-select-trigger" :model-value="languagePreference" :aria-label="t('界面语言')" @change="change" :options="[{ value: 'system', label: (t('跟随系统')) }, { value: 'zh-CN', label: '简体中文' }, { value: 'en-US', label: 'English' }]" />
  </label>
</template>
<style scoped>
.language-select { display:flex; align-items:center; flex:none; gap:4px; min-width:0; color:var(--muted); }
.language-select > svg { flex:none; }
/* Keep toolbar language selection lighter than form controls. */
.language-select :deep(button.language-select-trigger) {
  flex:none; width:max-content; height:28px; min-height:28px; gap:6px;
  padding:0 6px; border:0; border-radius:6px; background:transparent;
  color:var(--text); font-size:12px; font-weight:500; box-shadow:none; outline:none;
}
.language-select :deep(button.language-select-trigger:hover),
.language-select :deep(button.language-select-trigger[aria-expanded="true"]) { background:var(--panel2); }
.language-select :deep(button.language-select-trigger:focus) { border:0; box-shadow:none; }
.language-select :deep(button.language-select-trigger:focus-visible) { outline:2px solid var(--accent); outline-offset:2px; }
</style>
