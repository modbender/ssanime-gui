<template>
  <div class="min-h-screen bg-gray-100">
    <header class="bg-white shadow p-4">
      <div class="container mx-auto">
        <h1 class="text-2xl font-bold text-blue-600">SSAnime GUI</h1>
        <p class="text-gray-600">Anime Encoding Tool</p>
      </div>
    </header>

    <main class="container mx-auto py-6">
      <NuxtPage />
    </main>

    <footer class="bg-white shadow-inner p-4 mt-8">
      <div class="container mx-auto text-center text-sm text-gray-600">
        <p>App started at: {{ startTime }}</p>
      </div>
    </footer>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';

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
