<script setup>
import { useAppStore } from '../stores/app'
const store = useAppStore()
</script>

<template>
  <div class="flex flex-col items-center justify-center min-h-screen gap-6 p-8">
    <h1 class="text-3xl font-bold">Keepsake</h1>
    <p class="max-w-md text-center opacity-80">
      Log in with your Brightwheel account in a secure browser view. Your
      password is never seen by this app — only the session cookies are used.
    </p>
    <button class="btn btn-primary btn-lg" @click="store.login()">
      Log in with Brightwheel
    </button>
    <p class="text-sm opacity-60">
      After logging in, the app detects your session automatically and
      brings you back here.
    </p>
  </div>
</template>
