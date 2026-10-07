<script setup lang="ts">
const props = withDefaults(defineProps<{ open: boolean; title: string; closable?: boolean; cardClass?: string; showHeader?: boolean; inert?: boolean }>(), { closable: true, cardClass: '', showHeader: true, inert: false })
const emit = defineEmits<{ close: [] }>()
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="modal" role="presentation" @click.self="props.closable && emit('close')">
      <section class="modal-card" :class="props.cardClass" role="dialog" aria-modal="true" :aria-label="title" :inert="inert || undefined" :aria-hidden="inert || undefined">
        <template v-if="showHeader"><slot name="header"><h3>{{ title }}</h3></slot></template>
        <slot />
      </section>
    </div>
  </Teleport>
</template>
