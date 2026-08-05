<script setup>
import { useAppStore } from '../stores/app'
const store = useAppStore()
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-2">
      <h2 class="font-semibold">Students</h2>
      <label class="label cursor-pointer gap-2">
        <span class="label-text text-xs">All</span>
        <input type="checkbox" class="checkbox checkbox-sm" :checked="store.allSelected" @change="store.toggleAll()" />
      </label>
    </div>
    <div v-if="store.students.length === 0" class="text-sm opacity-60">No students found.</div>
    <label v-for="st in store.students" :key="st.object_id" class="label cursor-pointer justify-start gap-2">
      <input type="checkbox" class="checkbox checkbox-sm" v-model="store.selected[st.object_id]" />
      <span class="label-text" :class="{ 'demo-blur-text': store.demo }">{{ st.first_name }} {{ st.last_name }}</span>
    </label>
  </div>
</template>

<style scoped>
.demo-blur-text {
  filter: blur(6px);
}
</style>
