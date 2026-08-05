<script setup>
import { onMounted } from 'vue'
import { useAppStore } from './stores/app'
import LoginScreen from './components/LoginScreen.vue'
import MainScreen from './components/MainScreen.vue'

const store = useAppStore()
onMounted(() => store.init())
</script>

<template>
  <div class="min-h-screen bg-base-100 text-base-content">
    <LoginScreen v-if="!store.loggedIn" />
    <MainScreen v-else />
  </div>
</template>
