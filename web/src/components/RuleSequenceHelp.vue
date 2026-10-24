<script setup lang="ts">
import ProfileExtensionHelp from '@/components/ProfileExtensionHelp.vue'
defineProps<{ global?: boolean }>()
</script>

<template>
  <ProfileExtensionHelp kind="rules" :global="global" />
</template>
