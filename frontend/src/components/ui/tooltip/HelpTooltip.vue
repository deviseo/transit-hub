<script setup lang="ts">
import { CircleHelp } from 'lucide-vue-next'
import Tooltip from './Tooltip.vue'

defineProps<{
  text: string
}>()
</script>

<template>
  <Tooltip :text="text" wide>
    <button
      type="button"
      :aria-label="text"
      class="inline-flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-full text-muted-foreground/70 transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary"
    >
      <CircleHelp class="h-3.5 w-3.5" />
    </button>
  </Tooltip>
</template>
