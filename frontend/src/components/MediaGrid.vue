<script setup>
import { computed, ref } from 'vue'
import { useAppStore } from '../stores/app'
import MediaThumb from './MediaThumb.vue'

const store = useAppStore()
const pageSize = 60
const page = ref(0)

// Merge downloaded files with pending (not-yet-downloaded) remote items,
// keyed by expected filename so an item flips from grayscale to color in
// place when its download completes.
const items = computed(() => {
  const byKey = new Map()
  for (const f of store.files) {
    byKey.set(f.name, {
      key: f.name, name: f.name, file: f, url: null,
      pending: false, is_video: f.is_video,
      student_id: store.studentIdForFile(f.name),
    })
  }
  for (const p of store.pending) {
    if (byKey.has(p.expected_name)) continue // already downloaded
    byKey.set(p.expected_name, {
      key: p.expected_name, name: p.expected_name, file: null, url: p.url,
      pending: true, is_video: p.is_video, student_id: p.student_id,
      is_more_indicator: p.is_more_indicator, more_count: p.more_count,
    })
  }
  const selected = new Set(store.selectedIds)
  return [...byKey.values()].filter(
    (it) => !it.student_id || selected.has(it.student_id)
  )
})

const pendingCount = computed(() => items.value.filter((it) => it.pending).length)
const pageCount = computed(() => Math.max(1, Math.ceil(items.value.length / pageSize)))
const visible = computed(() => items.value.slice(0, (page.value + 1) * pageSize))
</script>

<template>
  <div>
    <div v-if="items.length === 0" class="flex flex-col items-center justify-center text-center py-20 gap-3 opacity-70">
      <template v-if="store.previewLoading">
        <span class="loading loading-spinner loading-md"></span>
        <span class="text-sm">Loading previews…</span>
      </template>
      <template v-else-if="!store.folder && store.pending.length === 0">
        <span class="text-4xl">🖼️</span>
        <span class="text-sm">Choose a download folder to see existing media.</span>
      </template>
      <template v-else>
        <span class="text-4xl">📷</span>
        <span class="text-sm">No media files yet.</span>
      </template>
    </div>

    <template v-else>
      <div class="flex items-baseline gap-2 mb-3 text-sm opacity-60">
        <span>{{ items.length }} item{{ items.length === 1 ? '' : 's' }}</span>
        <span v-if="pendingCount">· {{ pendingCount }} queued</span>
      </div>
      <div class="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 gap-2.5">
        <MediaThumb v-for="it in visible" :key="it.key" :item="it" />
      </div>
      <div v-if="page < pageCount - 1" class="flex justify-center p-4">
        <button class="btn btn-sm btn-ghost" @click="page++">Load more ({{ items.length - visible.length }} remaining)</button>
      </div>
    </template>
  </div>
</template>
