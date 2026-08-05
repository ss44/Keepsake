<script setup>
import { ref, watch } from 'vue'
import { useAppStore } from '../stores/app'
import StudentList from './StudentList.vue'
import ProgressBar from './ProgressBar.vue'
import { DotLottieVue } from '@lottiefiles/dotlottie-vue'
import MediaGrid from './MediaGrid.vue'
import loadingLottieUrl from '../loading.lottie?url'

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
    <header class="navbar bg-base-200 px-4 gap-2">
      <span class="font-bold text-lg flex-1">Keepsake</span>
      <button class="btn btn-ghost btn-sm" @click="store.logout()">Log out</button>
    </header>

    <div class="p-4 flex flex-wrap items-end gap-4 bg-base-200/50">
      <div class="form-control">
        <label class="label"><span class="label-text">Download folder</span></label>
        <div class="join">
          <input class="input input-bordered join-item w-72" readonly :value="store.folder" placeholder="No folder selected" />
          <button class="btn btn-primary join-item" @click="store.pickFolder()">Browse</button>
        </div>
      </div>

      <div class="form-control">
        <label class="label"><span class="label-text">From</span></label>
        <input type="date" class="input input-bordered" v-model="startDate" />
      </div>
      <div class="form-control">
        <label class="label"><span class="label-text">To</span></label>
        <input type="date" class="input input-bordered" v-model="endDate" />
      </div>

      <button
        class="btn btn-success"
        :disabled="!store.folder || store.selectedIds.length === 0 || store.downloading"
        @click="start"
      >Start download</button>
      <button v-if="store.downloading" class="btn btn-error" @click="store.cancel()">Cancel</button>
    </div>

    <div class="flex flex-1 min-h-0">
      <aside class="w-64 shrink-0 border-r border-base-300 overflow-y-auto p-3">
        <StudentList />
      </aside>
      <main class="flex-1 min-w-0 overflow-y-auto p-3 relative">
        <MediaGrid />
        <div v-if="store.downloading" class="absolute inset-0 bg-white/20 z-10 flex items-center justify-center pointer-events-none">
          <DotLottieVue :src="loadingLottieUrl" background="transparent" :speed="1" style="width: 250px; height: 250px;" loop autoplay />
        </div>
      </main>
    </div>

    <footer class="p-3 bg-base-200">
      <ProgressBar />
    </footer>
  </div>
</template>
