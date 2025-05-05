<template>
  <div
    v-if="visible"
    class="fixed bottom-0 left-0 right-0 p-4 bg-white shadow-lg border-t border-gray-200 z-50"
  >
    <div class="max-w-5xl mx-auto">
      <div class="flex justify-between items-center mb-2">
        <div>
          <span class="font-medium">{{ currentFile }}</span>
          <span class="ml-2 text-sm text-gray-600">{{ progressText }}</span>
        </div>
        <div class="flex items-center gap-4">
          <div class="text-sm text-gray-600">Speed: {{ speed }}</div>
          <div class="text-sm text-gray-600">ETA: {{ eta }}</div>
        </div>
      </div>

      <div class="w-full bg-gray-200 rounded-full h-3">
        <div
          class="bg-blue-600 h-3 rounded-full transition-all duration-300 ease-out"
          :style="{ width: `${progress}%` }"
        ></div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue';

const props = defineProps({
  visible: {
    type: Boolean,
    default: false,
  },
  progress: {
    type: Number,
    default: 0,
  },
  currentFile: {
    type: String,
    default: 'Processing...',
  },
  speed: {
    type: String,
    default: 'N/A',
  },
  eta: {
    type: String,
    default: 'Calculating...',
  },
});

const progressText = computed(() => {
  return `${props.progress.toFixed(1)}%`;
});
</script>
