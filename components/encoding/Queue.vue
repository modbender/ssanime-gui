<template>
  <div class="native-content">
    <div class="queue-container">
      <!-- Queue Header -->
      <div class="queue-header">
        <div class="queue-stats">
          <div class="stat-item">
            <span class="stat-label">Total Items</span>
            <span class="stat-value">{{ queueItems.length }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">Processing</span>
            <span class="stat-value">{{ processingCount }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">Completed</span>
            <span class="stat-value">{{ completedCount }}</span>
          </div>
        </div>
        <div class="queue-actions">
          <Button
            icon="pi pi-play"
            label="Start Queue"
            severity="success"
            size="small"
            :disabled="queueItems.length === 0"
          />
          <Button
            icon="pi pi-pause"
            label="Pause"
            severity="warning"
            size="small"
          />
          <Button
            icon="pi pi-trash"
            label="Clear"
            severity="danger"
            size="small"
            outlined
            @click="clearQueue"
          />
        </div>
      </div>

      <!-- Queue List -->
      <div class="queue-list">
        <div v-if="queueItems.length === 0" class="empty-state">
          <i class="pi pi-inbox empty-icon"></i>
          <h3>No items in queue</h3>
          <p>Add files to the encoder to see them here</p>
        </div>

        <div v-else class="queue-items">
          <div
            v-for="item in queueItems"
            :key="item.id"
            class="queue-item"
            :class="{
              processing: item.status === 'processing',
              completed: item.status === 'completed',
              error: item.status === 'error',
            }"
          >
            <div class="item-info">
              <div class="item-name">{{ item.fileName }}</div>
              <div class="item-details">
                <span class="item-profile">{{ item.profile }}</span>
                <span class="item-size">{{ formatFileSize(item.size) }}</span>
              </div>
            </div>

            <div class="item-progress">
              <ProgressBar
                v-if="item.status === 'processing'"
                :value="item.progress"
                :show-value="false"
                style="height: 6px"
              />
              <div
                v-else-if="item.status === 'completed'"
                class="status-completed"
              >
                <i class="pi pi-check"></i>
                <span>Completed</span>
              </div>
              <div v-else-if="item.status === 'error'" class="status-error">
                <i class="pi pi-exclamation-triangle"></i>
                <span>Error</span>
              </div>
              <div v-else class="status-pending">
                <i class="pi pi-clock"></i>
                <span>Pending</span>
              </div>
            </div>

            <div class="item-actions">
              <Button
                icon="pi pi-eye"
                severity="secondary"
                size="small"
                outlined
                v-tooltip="'View Details'"
              />
              <Button
                icon="pi pi-times"
                severity="danger"
                size="small"
                outlined
                v-tooltip="'Remove'"
                @click="removeItem(item.id)"
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
// No manual imports needed - Nuxt 3 auto-imports Vue and composables

// Queue items will be managed by the encoding service
const queueItems = ref([]);

// Computed properties
const processingCount = computed(
  () => queueItems.value.filter((item) => item.status === 'processing').length
);

const completedCount = computed(
  () => queueItems.value.filter((item) => item.status === 'completed').length
);

// Methods
const formatFileSize = (bytes) => {
  if (bytes === 0) return '0 Bytes';
  const k = 1024;
  const sizes = ['Bytes', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

const clearQueue = () => {
  queueItems.value = [];
};

const removeItem = (id) => {
  queueItems.value = queueItems.value.filter((item) => item.id !== id);
};
</script>

<style scoped>
.native-content {
  height: 100%;
  padding: 20px;
  overflow-y: auto;
}

.queue-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.queue-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px;
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 8px;
}

.queue-stats {
  display: flex;
  gap: 24px;
}

.stat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.stat-label {
  font-size: 12px;
  color: var(--text-secondary);
  margin-bottom: 2px;
}

.stat-value {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.queue-actions {
  display: flex;
  gap: 8px;
}

.queue-list {
  flex: 1;
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  overflow: hidden;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 300px;
  color: var(--text-secondary);
}

.empty-icon {
  font-size: 48px;
  margin-bottom: 16px;
  opacity: 0.5;
}

.empty-state h3 {
  margin: 0 0 8px 0;
  font-size: 18px;
}

.empty-state p {
  margin: 0;
  font-size: 14px;
}

.queue-items {
  height: 100%;
  overflow-y: auto;
}

.queue-item {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-color);
  transition: background-color 0.15s ease;
}

.queue-item:hover {
  background: var(--bg-hover);
}

.queue-item:last-child {
  border-bottom: none;
}

.queue-item.processing {
  background: rgba(34, 197, 94, 0.05);
}

.queue-item.completed {
  background: rgba(59, 130, 246, 0.05);
}

.queue-item.error {
  background: rgba(239, 68, 68, 0.05);
}

.item-info {
  flex: 1;
  min-width: 0;
}

.item-name {
  font-weight: 500;
  color: var(--text-primary);
  font-size: 14px;
  margin-bottom: 4px;
  text-overflow: ellipsis;
  white-space: nowrap;
  overflow: hidden;
}

.item-details {
  display: flex;
  gap: 12px;
  font-size: 12px;
  color: var(--text-secondary);
}

.item-progress {
  flex: 0 0 200px;
  margin: 0 16px;
}

.status-completed,
.status-error,
.status-pending {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 500;
}

.status-completed {
  color: var(--success-color);
}

.status-error {
  color: var(--error-color);
}

.status-pending {
  color: var(--text-secondary);
}

.item-actions {
  display: flex;
  gap: 6px;
}
</style>
