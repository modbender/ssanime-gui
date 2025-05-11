<template>
  <Card
    is="footer"
    class="bg-card shadow-inner border-t border-border p-4 mt-8"
  >
    <div class="container mx-auto text-center text-sm text-muted-foreground">
      <p>App started at: {{ startTime }}</p>
    </div>
  </Card>
</template>

<script setup>
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

<style></style>
