<script setup>
import { onMounted, ref, watch } from 'vue'
import { useAppStore } from '../stores/app'

const props = defineProps({ item: Object })
const store = useAppStore()
const src = ref(null)
const loaded = ref(false)
const el = ref(null)

// Pending items use the remote URL directly; downloaded items get a
// locally generated thumbnail. The pending->downloaded transition keeps
// the same <img> element so the CSS filter transition animates.
watch(
  () => [props.item.pending, props.item.url, props.item.file?.path],
  () => {
    src.value = null
    loaded.value = false
    load()
  }
)

function load() {
  if (props.item.pending) {
    src.value = props.item.url
    return
  }
  if (props.item.is_video || !props.item.file) {
    // Videos render an icon rather than a thumbnail; skip the shimmer.
    if (props.item.is_video) loaded.value = true
    return
  }
  store.thumb(props.item.file).then((t) => { src.value = t })
}

onMounted(() => {
  const io = new IntersectionObserver((entries) => {
    if (entries.some((e) => e.isIntersecting)) {
      io.disconnect()
      load()
    }
  })
  io.observe(el.value)
})
</script>

<template>
  <figure
    ref="el"
    class="aspect-square bg-base-300 rounded-lg overflow-hidden relative ring-1 ring-base-content/5 transition-shadow hover:ring-base-content/20"
    :title="item.name"
  >
    <div v-if="item.is_more_indicator" class="flex flex-col items-center justify-center h-full opacity-60 bg-base-200">
      <span class="text-2xl font-bold">+{{ item.more_count }}</span>
      <span class="text-xs uppercase tracking-wider mt-1">more</span>
    </div>

    <template v-else>
      <!-- Shimmer skeleton placeholder shown until the thumbnail is ready. -->
      <div v-if="!loaded" class="absolute inset-0 skeleton-shimmer"></div>

      <img
        v-if="src"
        :src="src"
        class="w-full h-full object-cover thumb-img"
        :class="{
          pending: item.pending,
          'demo-blur': store.demo && item.pending,
          revealed: loaded,
        }"
        loading="lazy"
        @load="loaded = true"
        @error="loaded = true"
      />
      <div
        v-else-if="item.is_video"
        class="flex items-center justify-center h-full text-4xl"
        :class="{ 'opacity-40': item.pending }"
      >🎬</div>
    </template>

    <span
      v-if="item.pending && !item.is_more_indicator"
      class="absolute top-1.5 left-1.5 flex items-center gap-1 text-[10px] font-medium uppercase tracking-wider bg-base-100/70 backdrop-blur px-1.5 py-0.5 rounded"
    >
      <span class="pending-dot"></span> queued
    </span>
  </figure>
</template>

<style scoped>
/* Placeholder shimmer while a thumbnail is being generated/downloaded. */
.skeleton-shimmer {
  background: linear-gradient(
    100deg,
    hsl(var(--b3)) 40%,
    hsl(var(--b2)) 50%,
    hsl(var(--b3)) 60%
  );
  background-size: 200% 100%;
  animation: shimmer 1.4s ease-in-out infinite;
}
@keyframes shimmer {
  to { background-position: -200% 0; }
}

/* Reveal animation: start slightly blurred/scaled, settle once loaded. */
.thumb-img {
  opacity: 0;
  transform: scale(1.04);
  filter: blur(8px) grayscale(0);
  transition:
    opacity 0.5s ease,
    transform 0.6s ease,
    filter 1.2s ease;
}
.thumb-img.revealed {
  opacity: 1;
  transform: scale(1);
  filter: blur(0) grayscale(0);
}
/* Pending (not yet downloaded) items sit desaturated; when the download
   completes the same element transitions to full color in place. */
.thumb-img.pending.revealed {
  filter: blur(0) grayscale(1) brightness(0.65);
}
/* Demo mode: pending previews load from the remote URL (bypassing the
   pixelating backend), so blur them heavily instead. */
.thumb-img.pending.demo-blur.revealed {
  filter: grayscale(1) brightness(0.65) blur(24px);
}

.pending-dot {
  width: 6px;
  height: 6px;
  border-radius: 9999px;
  background: hsl(var(--su));
  animation: pulse 1.2s ease-in-out infinite;
}
@keyframes pulse {
  50% { opacity: 0.3; }
}
</style>
