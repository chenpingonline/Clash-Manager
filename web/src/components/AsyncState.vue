<script setup lang="ts">
import { t } from '@/services/i18n'

defineProps<{ loading?: boolean; error?: string; empty?: string }>()
</script>

<template>
  <div v-if="loading" class="empty">{{ t("加载中…") }}</div>
  <div v-else-if="error" class="empty error-text">{{ error }}</div>
  <div v-else-if="empty" class="empty">{{ empty }}</div>
  <slot v-else />
</template>
