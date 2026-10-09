<script setup lang="ts">
import { languagePreference, setLanguage, t, type LanguagePreference } from '@/services/i18n'
function change(event: Event) { setLanguage((event.target as HTMLSelectElement).value as LanguagePreference) }
</script>
<template>
  <label class="language-select">
    <span>{{ t('界面语言') }}</span>
    <select :value="languagePreference" :aria-label="t('界面语言')" @change="change">
      <option value="system">{{ t('跟随系统') }}</option>
      <option value="zh-CN">简体中文</option>
      <option value="en-US">English</option>
    </select>
  </label>
</template>
<style scoped>
.language-select { display:flex; align-items:center; flex:none; gap:8px; min-width:0; font-size:12px; color:var(--muted); }
.language-select > span { white-space:nowrap; }
.language-select select { width:132px; height:28px; padding:4px 8px; font-size:12px; }
@media(max-width:680px) {
  .language-select { gap:5px; }
  .language-select select { width:112px; }
}
</style>
