<script setup lang="ts">
import { computed } from 'vue'
import { logMessageParts } from '@/services/log-highlight'

const props = defineProps<{ text: string; query: string }>()
const parts = computed(() => logMessageParts(props.text, props.query))
</script>

<template>
  <span class="log-message"><template v-for="(part, index) in parts" :key="index"><mark v-if="part.match">{{ part.text }}</mark><span v-else-if="part.kind" :class="`log-token-${part.kind}`">{{ part.text }}</span><template v-else>{{ part.text }}</template></template></span>
</template>
