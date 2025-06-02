<template>
  <div class="native-content">
    <div class="logs-container">
      <!-- Logs Header -->
      <div class="logs-header">
        <div class="logs-info">
          <h3>System Logs</h3>
          <span class="log-count">{{ logs.length }} entries</span>
        </div>
        <div class="logs-actions">
          <Dropdown
            v-model="selectedLevel"
            :options="logLevels"
            optionLabel="label"
            optionValue="value"
            placeholder="Filter by level"
            style="width: 150px"
          />
          <Button
            icon="pi pi-refresh"
            label="Refresh"
            severity="secondary"
            size="small"
            @click="refreshLogs"
          />
          <Button
            icon="pi pi-download"
            label="Export"
            severity="info"
            size="small"
            outlined
          />
          <Button
            icon="pi pi-trash"
            label="Clear"
            severity="danger"
            size="small"
            outlined
            @click="clearLogs"
          />
        </div>
      </div>

      <!-- Logs Content -->
      <div class="logs-content">
        <div class="logs-list">
          <div
            v-for="log in filteredLogs"
            :key="log.id"
            class="log-entry"
            :class="`log-${log.level}`"
          >
            <div class="log-timestamp">
              {{ formatTimestamp(log.timestamp) }}
            </div>
            <div class="log-level">
              <Tag
                :value="log.level.toUpperCase()"
                :severity="getLogSeverity(log.level)"
              />
            </div>
            <div class="log-message">{{ log.message }}</div>
            <div class="log-source" v-if="log.source">{{ log.source }}</div>
          </div>
        </div>

        <div v-if="filteredLogs.length === 0" class="empty-logs">
          <i class="pi pi-file-edit empty-icon"></i>
          <h3>No logs found</h3>
          <p>No log entries match the current filter</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
// No manual imports needed - Nuxt 3 auto-imports Vue and composables

// Log levels
const logLevels = [
  { label: 'All Levels', value: null },
  { label: 'Info', value: 'info' },
  { label: 'Warning', value: 'warning' },
  { label: 'Error', value: 'error' },
  { label: 'Debug', value: 'debug' },
];

const selectedLevel = ref(null);

// Sample log data
const logs = ref([
  {
    id: 1,
    timestamp: new Date(Date.now() - 1000 * 60 * 5),
    level: 'info',
    message: 'Application started successfully',
    source: 'Main Process',
  },
  {
    id: 2,
    timestamp: new Date(Date.now() - 1000 * 60 * 3),
    level: 'info',
    message: 'FFmpeg encoder initialized',
    source: 'Encoder Service',
  },
  {
    id: 3,
    timestamp: new Date(Date.now() - 1000 * 60 * 2),
    level: 'warning',
    message: 'Output directory not found, creating new directory',
    source: 'File System',
  },
  {
    id: 4,
    timestamp: new Date(Date.now() - 1000 * 60 * 1),
    level: 'error',
    message: 'Failed to load encoding profile: profile.json not found',
    source: 'Profile Manager',
  },
  {
    id: 5,
    timestamp: new Date(),
    level: 'debug',
    message: 'Memory usage: 245MB, CPU usage: 15%',
    source: 'System Monitor',
  },
]);

// Computed properties
const filteredLogs = computed(() => {
  if (!selectedLevel.value) {
    return logs.value;
  }
  return logs.value.filter((log) => log.level === selectedLevel.value);
});

// Methods
const formatTimestamp = (timestamp) => {
  return timestamp.toLocaleTimeString() + ' ' + timestamp.toLocaleDateString();
};

const getLogSeverity = (level) => {
  switch (level) {
    case 'error':
      return 'danger';
    case 'warning':
      return 'warning';
    case 'info':
      return 'info';
    case 'debug':
      return 'secondary';
    default:
      return 'info';
  }
};

const refreshLogs = () => {
  // Simulate loading new logs
  const newLog = {
    id: logs.value.length + 1,
    timestamp: new Date(),
    level: 'info',
    message: 'Logs refreshed',
    source: 'Log Viewer',
  };
  logs.value.unshift(newLog);
};

const clearLogs = () => {
  logs.value = [];
};

onMounted(() => {
  // Auto-scroll to bottom
  setTimeout(() => {
    const logsList = document.querySelector('.logs-list');
    if (logsList) {
      logsList.scrollTop = logsList.scrollHeight;
    }
  }, 100);
});
</script>

<style scoped>
.native-content {
  height: 100%;
  padding: 20px;
  overflow: hidden;
}

.logs-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.logs-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px;
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 8px;
}

.logs-info h3 {
  margin: 0 0 4px 0;
  color: var(--text-primary);
}

.log-count {
  font-size: 12px;
  color: var(--text-secondary);
}

.logs-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.logs-content {
  flex: 1;
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  overflow: hidden;
  position: relative;
}

.logs-list {
  height: 100%;
  overflow-y: auto;
  padding: 8px;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.4;
}

.log-entry {
  display: grid;
  grid-template-columns: 140px 80px 1fr auto;
  gap: 12px;
  padding: 6px 8px;
  border-radius: 4px;
  margin-bottom: 2px;
  align-items: center;
}

.log-entry:hover {
  background: var(--bg-hover);
}

.log-timestamp {
  font-size: 11px;
  color: var(--text-muted);
  white-space: nowrap;
}

.log-level {
  flex-shrink: 0;
}

.log-message {
  color: var(--text-primary);
  word-break: break-word;
}

.log-source {
  font-size: 11px;
  color: var(--text-muted);
  text-align: right;
  white-space: nowrap;
}

.log-info {
  border-left: 3px solid var(--info-color);
}

.log-warning {
  border-left: 3px solid var(--warning-color);
  background: rgba(234, 88, 12, 0.05);
}

.log-error {
  border-left: 3px solid var(--error-color);
  background: rgba(239, 68, 68, 0.05);
}

.log-debug {
  border-left: 3px solid var(--text-muted);
  opacity: 0.8;
}

.empty-logs {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--text-secondary);
}

.empty-icon {
  font-size: 48px;
  margin-bottom: 16px;
  opacity: 0.5;
}

.empty-logs h3 {
  margin: 0 0 8px 0;
  font-size: 18px;
}

.empty-logs p {
  margin: 0;
  font-size: 14px;
}

/* Scrollbar styling */
.logs-list::-webkit-scrollbar {
  width: 8px;
}

.logs-list::-webkit-scrollbar-track {
  background: var(--bg-secondary);
}

.logs-list::-webkit-scrollbar-thumb {
  background: var(--border-color);
  border-radius: 4px;
}

.logs-list::-webkit-scrollbar-thumb:hover {
  background: var(--border-hover);
}
</style>
