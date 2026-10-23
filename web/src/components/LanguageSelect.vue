<script setup lang="ts">
import { Languages } from 'lucide-vue-next'
import AppSelect from '@/components/AppSelect.vue'
import { languagePreference, setLanguage, t, type LanguagePreference } from '@/services/i18n'
function change(value: LanguagePreference) { setLanguage(value) }
</script>
<template>
  <label class="language-select">
    <Languages :size="15" aria-hidden="true" />
    <AppSelect :model-value="languagePreference" :aria-label="t('界面语言')" @change="change" :options="[{ value: 'system', label: (t('跟随系统')) }, { value: 'zh-CN', label: '简体中文' }, { value: 'en-US', label: 'English' }]" />
  </label>
</template>
<style scoped>
.language-select { display:flex; align-items:center; flex:none; gap:8px; min-width:0; font-size:12px; color:var(--muted); }
.language-select > span { white-space:nowrap; }
.language-select .app-select { width:132px; height:28px; padding:4px 8px; font-size:12px; }
@media(max-width:680px) {
  .language-select { gap:5px; }
  .language-select .app-select { width:112px; }
}
</style>
