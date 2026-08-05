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
    <progress
      class="progress flex-1 min-w-0 transition-all"
      :class="store.downloading ? 'progress-success' : 'progress-primary'"
      :value="pct"
      max="100"
    ></progress>
    <span class="text-xs tabular-nums whitespace-nowrap shrink-0 opacity-80">
      <template v-if="store.progress.total > 0">{{ store.progress.done }} / {{ store.progress.total }} · {{ pct }}%</template>
      <template v-else>Idle</template>
    </span>
    <span class="text-xs opacity-60 truncate shrink max-w-[40%]">{{ store.status }}</span>
  </div>
</template>
