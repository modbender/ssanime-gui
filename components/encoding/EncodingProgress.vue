<template>
  <div
    v-if="visible"
    class="fixed bottom-0 left-0 right-0 p-4 bg-card shadow-lg border-t border-border z-50"
  >
    <div class="max-w-5xl mx-auto">
      <div class="flex justify-between items-center mb-2">
        <div>
          <span class="font-medium">{{ currentFile }}</span>
          <Badge variant="outline" class="ml-2">{{ progressText }}</Badge>
        </div>
        <div class="flex items-center gap-4">
          <div class="flex items-center text-sm text-muted-foreground">
            <Icon name="tabler:gauge" class="mr-1 h-4 w-4" />
            <span>{{ speed }}</span>
          </div>
          <div class="flex items-center text-sm text-muted-foreground">
            <Icon name="tabler:clock" class="mr-1 h-4 w-4" />
            <span>{{ eta }}</span>
          </div>
          <Button v-if="onCancel" variant="outline" size="sm" @click="onCancel">
            <Icon name="tabler:x" class="mr-1 h-4 w-4" />
            Cancel
          </Button>
        </div>
      </div>

      <Progress :value="progress" class="w-full h-2" />
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue';

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
  onCancel: {
    type: Function,
    default: null,
  },
});

const progressText = computed(() => {
  return `${props.progress.toFixed(1)}%`;
});
</script>
