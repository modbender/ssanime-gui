<template>
  <div class="min-h-screen bg-background text-foreground">
    <header class="bg-card shadow-sm border-b border-border p-4">
      <div class="container mx-auto">
        <h1 class="text-2xl font-bold text-primary">SSAnime GUI</h1>
        <p class="text-muted-foreground">Anime Encoding Tool</p>
      </div>
    </header>

    <main class="container mx-auto py-6">
      <NuxtPage />
    </main>

    <footer class="bg-card shadow-inner border-t border-border p-4 mt-8">
      <div class="container mx-auto text-center text-sm text-muted-foreground">
        <p>App started at: {{ startTime }}</p>
      </div>
    </footer>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';
import { Icon } from '#components';

const startTime = ref('Loading...');

onMounted(async () => {
  try {
    const time = await window.ipcRenderer.invoke('app-start-time');
    console.log('App start time:', time);
    startTime.value = time;
  } catch (error) {
    console.error('Failed to get app start time:', error);
    startTime.value = 'Error loading time';
  }
});
</script>

<style>
.nuxt-icon svg {
  display: inline-block;
}
</style>
