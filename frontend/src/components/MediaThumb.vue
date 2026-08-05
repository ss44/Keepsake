<script setup>
import { onMounted, ref, watch } from 'vue'
import { useAppStore } from '../stores/app'

const props = defineProps({ item: Object })
const store = useAppStore()
const src = ref(null)
const el = ref(null)

// Pending items use the remote URL directly; downloaded items get a
// locally generated thumbnail. The pending->downloaded transition keeps
// the same <img> element so the CSS filter transition animates.
watch(
  () => [props.item.pending, props.item.url, props.item.file?.path],
  () => {
    src.value = null
    load()
  }
)

function load() {
  if (props.item.pending) {
    src.value = props.item.url
    return
  }
  if (props.item.is_video || !props.item.file) return
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
  <figure ref="el" class="aspect-square bg-base-300 rounded overflow-hidden relative" :title="item.name">
    <div v-if="item.is_more_indicator" class="flex flex-col items-center justify-center h-full opacity-60 bg-base-200">
      <span class="text-2xl font-bold">+{{ item.more_count }}</span>
      <span class="text-xs uppercase tracking-wider mt-1">more</span>
    </div>
    <img
      v-else-if="src"
      :src="src"
      class="w-full h-full object-cover thumb-img"
      :class="{ pending: item.pending, 'demo-blur': store.demo && item.pending }"
      loading="lazy"
    />
    <div v-else-if="item.is_video" class="flex items-center justify-center h-full text-4xl" :class="{ 'opacity-40': item.pending }">🎬</div>
    <div v-else class="flex items-center justify-center h-full text-sm opacity-40">…</div>
    <span v-if="item.pending" class="badge badge-sm absolute top-1 left-1 opacity-80">pending</span>
  </figure>
</template>

<style scoped>
.thumb-img {
  filter: grayscale(0);
  transition: filter 1.2s ease;
}
.thumb-img.pending {
  filter: grayscale(1) brightness(0.65);
}
/* Demo mode: pending previews load from the remote URL (bypassing the
   pixelating backend), so blur them heavily instead. */
.thumb-img.pending.demo-blur {
  filter: grayscale(1) brightness(0.65) blur(24px);
}
.demo-blur-text {
  filter: blur(6px);
}
</style>
