<template>
  <div class="native-page-content">
    <!-- Render different components based on the current view -->
    <component :is="currentComponent" />
  </div>
</template>

<script setup>
import { computed, ref } from 'vue';

// Define the component mapping (components are auto-imported by Nuxt)
const componentMap = {
  encoder: 'EncoderMain',
  profiles: 'EncodingProfiles',
  queue: 'EncodingQueue',
  logs: 'SystemLogs',
  settings: 'AppSettings',
};

// For now, default to encoder since we don't have the view state management yet
// This will be updated once the parent app passes the current view
const currentView = ref('encoder');

const currentComponent = computed(() => {
  return componentMap[currentView.value] || 'EncoderMain';
});
</script>

<style scoped>
.native-page-content {
  height: 100%;
  background: var(--bg-primary);
  overflow: hidden;
}

/* Remove any web-style padding/margins */
.native-page-content > * {
  height: 100%;
}
</style>
