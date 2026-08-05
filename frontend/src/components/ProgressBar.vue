<script setup>
import { computed } from 'vue'
import { useAppStore } from '../stores/app'
const store = useAppStore()
const pct = computed(() =>
  store.progress.total > 0 ? Math.round((store.progress.done / store.progress.total) * 100) : 0
)
</script>

<template>
  <div class="flex items-center gap-3 w-full">
    <progress class="progress progress-primary flex-1 min-w-0" :value="pct" max="100"></progress>
    <span class="text-sm whitespace-nowrap shrink-0">{{ store.progress.done }} / {{ store.progress.total }}</span>
    <span class="text-sm opacity-70 truncate shrink max-w-[40%]">{{ store.status }}</span>
  </div>
</template>
