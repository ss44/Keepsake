<script setup>
import { useAppStore } from '../stores/app'
const store = useAppStore()

function initials(st) {
  return ((st.first_name?.[0] || '') + (st.last_name?.[0] || '')).toUpperCase() || '?'
}
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-2 px-1">
      <h2 class="text-xs font-semibold uppercase tracking-wider opacity-60">Students</h2>
      <label class="flex items-center cursor-pointer gap-2">
        <span class="text-xs opacity-60">All</span>
        <input type="checkbox" class="checkbox checkbox-sm checkbox-primary" :checked="store.allSelected" @change="store.toggleAll()" />
      </label>
    </div>
    <div v-if="store.students.length === 0" class="text-sm opacity-60 px-1">No students found.</div>
    <label
      v-for="st in store.students"
      :key="st.object_id"
      class="flex items-center gap-2.5 px-2 py-2 rounded-lg cursor-pointer transition-colors hover:bg-base-200"
      :class="{ 'opacity-50': !store.selected[st.object_id] }"
    >
      <input type="checkbox" class="checkbox checkbox-sm checkbox-primary" v-model="store.selected[st.object_id]" />
      <span class="w-7 h-7 shrink-0 rounded-full bg-primary/20 text-primary flex items-center justify-center text-xs font-semibold">
        {{ initials(st) }}
      </span>
      <span class="text-sm truncate" :class="{ 'demo-blur-text': store.demo }">{{ st.first_name }} {{ st.last_name }}</span>
    </label>
  </div>
</template>

<style scoped>
.demo-blur-text {
  filter: blur(6px);
}
</style>
