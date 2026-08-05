<script setup>
import { ref, watch } from 'vue'
import { useAppStore } from '../stores/app'
import StudentList from './StudentList.vue'
import ProgressBar from './ProgressBar.vue'
import MediaGrid from './MediaGrid.vue'
import logoUrl from '../assets/keepsake.svg'

const store = useAppStore()
const startDate = ref('2021-02-04')
const endDate = ref(new Date().toISOString().split('T')[0])

let previewTimer = null
watch(
  () => [store.selectedIds.join(','), startDate.value, endDate.value],
  () => {
    clearTimeout(previewTimer)
    previewTimer = setTimeout(() => {
      if (!store.downloading) store.loadPreview(startDate.value, endDate.value)
    }, 500)
  }
)

// Refresh previews after a download run so remaining items stay accurate.
watch(
  () => store.downloading,
  (dl, was) => {
    if (was && !dl) store.loadPreview(startDate.value, endDate.value)
  }
)

function start() {
  store.startDownload(startDate.value, endDate.value)
}
</script>

<template>
  <div class="flex flex-col h-screen">
    <header class="navbar bg-base-200 border-b border-base-300 px-4 gap-3 min-h-14">
      <img :src="logoUrl" alt="Keepsake" class="w-7 h-7" />
      <span class="font-bold text-lg flex-1 tracking-tight">Keepsake</span>
      <button class="btn btn-ghost btn-sm opacity-70 hover:opacity-100" @click="store.logout()">Log out</button>
    </header>

    <div class="px-4 py-3 flex flex-wrap items-end gap-x-5 gap-y-3 bg-base-200/50 border-b border-base-300">
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs uppercase tracking-wider opacity-60">Download folder</span></label>
        <div class="join">
          <input class="input input-bordered input-sm join-item w-72" readonly :value="store.folder" placeholder="No folder selected" />
          <button class="btn btn-primary btn-sm join-item" @click="store.pickFolder()">Browse</button>
        </div>
      </div>

      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs uppercase tracking-wider opacity-60">From</span></label>
        <input type="date" class="input input-bordered input-sm" v-model="startDate" />
      </div>
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs uppercase tracking-wider opacity-60">To</span></label>
        <input type="date" class="input input-bordered input-sm" v-model="endDate" />
      </div>

      <div class="flex gap-2 ml-auto">
        <button
          class="btn btn-success btn-sm"
          :disabled="!store.folder || store.selectedIds.length === 0 || store.downloading"
          @click="start"
        >Start download</button>
        <button v-if="store.downloading" class="btn btn-error btn-sm" @click="store.cancel()">Cancel</button>
      </div>
    </div>

    <div class="flex flex-1 min-h-0">
      <aside class="w-64 shrink-0 border-r border-base-300 overflow-y-auto p-3">
        <StudentList />
      </aside>
      <main class="flex-1 min-w-0 overflow-y-auto p-4 relative">
        <MediaGrid />
        <!-- Unobtrusive downloading indicator; the grid stays fully usable
             and thumbnails animate in as files land. -->
        <div
          v-if="store.downloading"
          class="sticky bottom-3 z-10 flex justify-center pointer-events-none"
        >
          <div class="flex items-center gap-2 bg-base-100/90 backdrop-blur shadow-lg rounded-full px-4 py-2 text-sm">
            <span class="loading loading-spinner loading-xs text-success"></span>
            Downloading… {{ store.progress.done }} / {{ store.progress.total }}
          </div>
        </div>
      </main>
    </div>

    <footer class="px-4 py-2.5 bg-base-200 border-t border-base-300">
      <ProgressBar />
    </footer>
  </div>
</template>
