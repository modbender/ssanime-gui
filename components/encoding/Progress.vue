<template>
  <div v-if="visible">
    <ProgressBar :value="progress" :show-value="true" style="height: 1.5rem" />
    <div class="mt-2">
      <p>Current File: {{ currentFile }}</p>
      <p>Speed: {{ speed }} | ETA: {{ eta }}</p>
      <Button
        label="Cancel"
        icon="pi pi-times"
        severity="danger"
        @click="onCancel"
      />
    </div>
  </div>
</template>

<script setup>
const props = defineProps({
  visible: Boolean,
  progress: Number,
  currentFile: String,
  speed: String,
  eta: String,
  onCancel: Function,
});
</script>
