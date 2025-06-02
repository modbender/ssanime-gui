<template>
  <div class="footer">
    <span>App started at: {{ startTime }}</span>
  </div>
</template>

<script setup>
// No manual imports needed - Nuxt 3 auto-imports Vue and composables

const startTime = ref('Loading...');

onMounted(async () => {
  try {
    const time = await window.ipcRenderer.invoke('app-start-time');
    startTime.value = time;
  } catch (error) {
    startTime.value = 'Error loading time';
  }
});
</script>

<style scoped>
.footer {
  text-align: center;
  color: #b0b6c3;
  font-size: 0.95rem;
  padding: 12px 0 0 0;
  margin-top: 12px;
}
</style>
