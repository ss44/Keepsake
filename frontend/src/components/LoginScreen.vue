<script setup>
import { useAppStore } from '../stores/app'
import logoUrl from '../assets/keepsake.svg'
const store = useAppStore()
</script>

<template>
  <div class="flex flex-col items-center justify-center min-h-screen p-8 bg-gradient-to-b from-base-200 to-base-100">
    <div class="card w-full max-w-md bg-base-200/60 shadow-xl border border-base-300">
      <div class="card-body items-center text-center gap-4">
        <img :src="logoUrl" alt="Keepsake" class="w-20 h-20" />
        <h1 class="text-3xl font-bold tracking-tight">Keepsake</h1>
        <p class="opacity-70 text-sm leading-relaxed">
          Log in with your Brightwheel account in a secure browser view. Your
          password is never seen by this app — only the session cookies are used.
        </p>
        <button class="btn btn-primary btn-lg w-full mt-2" @click="store.login()">
          Log in with Brightwheel
        </button>
        <p class="text-xs opacity-50">
          After logging in, the app detects your session automatically and
          brings you back here.
        </p>
      </div>
    </div>
  </div>
</template>
