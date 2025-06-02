<template>
  <div class="native-encoder-container">
    <!-- File Selection Section -->
    <Card class="native-card">
      <template #header>
        <div class="native-card-header">
          <i class="pi pi-file-video header-icon"></i>
          <h3>Input Files</h3>
        </div>
      </template>
      <template #content>
        <div class="file-selection-content">
          <Button
            label="Select Files"
            icon="pi pi-folder-open"
            @click="selectFiles"
            :loading="isLoadingFiles"
            severity="info"
            size="small"
            v-tooltip.top="'Choose video files to encode'"
          />

          <div v-if="selectedFiles.length > 0" class="selected-files-info">
            <div class="files-count">
              <Tag
                :value="`${selectedFiles.length} files selected`"
                severity="success"
                class="files-tag"
              />
            </div>
            <div class="files-preview">
              <div
                v-for="(file, index) in selectedFiles.slice(0, 3)"
                :key="index"
                class="file-preview-item"
              >
                <i class="pi pi-file"></i>
                <span>{{ getFileName(file) }}</span>
              </div>
              <div v-if="selectedFiles.length > 3" class="more-files-indicator">
                <span>+{{ selectedFiles.length - 3 }} more files</span>
              </div>
            </div>
          </div>
        </div>
      </template>
    </Card>

    <!-- Output Directory Card -->
    <Card class="selection-card">
      <template #header>
        <div class="card-header">
          <i class="pi pi-folder header-icon"></i>
          <h3>Output Directory</h3>
        </div>
      </template>
      <template #content>
        <div class="output-selection">
          <div class="directory-input-group">
            <InputText
              v-model="outputDirectory"
              placeholder="Select output directory..."
              readonly
              class="directory-input"
            />
            <Button
              icon="pi pi-folder-open"
              @click="selectOutputDirectory"
              :loading="isLoadingOutput"
              severity="secondary"
              size="small"
              v-tooltip.top="'Choose output directory'"
            />
          </div>

          <div v-if="outputDirectory" class="directory-info">
            <div class="directory-path">
              <i class="pi pi-folder-open"></i>
              <span>{{ outputDirectory }}</span>
            </div>
          </div>
        </div>
      </template>
    </Card>

    <!-- Encoding Controls Card -->
    <Card class="encoder-card">
      <template #header>
        <div class="card-header">
          <i class="pi pi-cog header-icon"></i>
          <h3>Encoding Controls</h3>
        </div>
      </template>
      <template #content>
        <div class="encoder-content">
          <div class="status-section">
            <div class="status-indicator" :class="statusClass">
              <i :class="statusIcon"></i>
              <span>{{ statusText }}</span>
            </div>
          </div>

          <div class="encoder-actions">
            <Button
              label="Start Encoding"
              icon="pi pi-play"
              :disabled="!canStartEncoding || isEncoding"
              :loading="isStarting"
              @click="startEncoding"
              severity="success"
              v-tooltip.top="
                canStartEncoding
                  ? 'Start the encoding process'
                  : 'Select files and output directory first'
              "
              class="action-btn"
            />
            <Button
              label="Stop Encoding"
              icon="pi pi-stop"
              severity="danger"
              :disabled="!isEncoding"
              @click="stopEncoding"
              v-tooltip.top="'Stop the current encoding process'"
              class="action-btn"
            />
            <Button
              label="Clear All"
              icon="pi pi-times"
              severity="secondary"
              outlined
              @click="clearAll"
              :disabled="isEncoding"
              v-tooltip.top="'Clear all selections'"
              class="action-btn"
            />
          </div>

          <div class="active-profile">
            <span>Active Profile:</span>
            <Tag
              :value="activeProfile || 'None Selected'"
              :severity="activeProfile ? 'info' : 'warn'"
            />
          </div>

          <div v-if="isEncoding" class="encoding-progress">
            <div class="progress-header">
              <h4>Encoding Progress</h4>
              <Button
                icon="pi pi-eye"
                @click="viewLogs"
                severity="secondary"
                text
                rounded
                size="small"
                v-tooltip.top="'View encoding logs'"
              />
            </div>
            <ProgressBar
              :value="encodingProgress.percent"
              :showValue="true"
              class="main-progress"
            />
            <div class="progress-details">
              <div class="detail-item">
                <i class="pi pi-file"></i>
                <span>{{
                  encodingProgress.currentFile || 'Preparing...'
                }}</span>
              </div>
              <div class="detail-item">
                <i class="pi pi-clock"></i>
                <span
                  >Speed: {{ encodingProgress.speed || '0x' }} | ETA:
                  {{ encodingProgress.eta || 'Calculating...' }}</span
                >
              </div>
            </div>
          </div>

          <div v-if="logFilePath" class="log-section">
            <div class="log-input-group">
              <InputText
                v-model="logFilePath"
                readonly
                placeholder="Log file path"
              />
              <Button
                icon="pi pi-copy"
                @click="copyLogPathToClipboard"
                severity="info"
                v-tooltip.top="'Copy log file path'"
              />
            </div>
          </div>
        </div>
      </template>
    </Card>

    <!-- Encoding Profiles Card -->
    <Card class="profiles-card">
      <template #header>
        <div class="card-header">
          <i class="pi pi-sliders-h header-icon"></i>
          <h3>Encoding Profiles</h3>
        </div>
      </template>
      <template #content>
        <EncodingProfiles @profile-changed="updateActiveProfile" />
      </template>
    </Card>
  </div>
</template>

<script setup>
// No manual imports needed - Nuxt 3 auto-imports Vue and composables

const toast = useToast();

// State
const selectedFiles = ref([]);
const outputDirectory = ref('');
const isEncoding = ref(false);
const isStarting = ref(false);
const isLoadingFiles = ref(false);
const isLoadingOutput = ref(false);
const encodingProgress = ref({
  percent: 0,
  currentFile: '',
  speed: '',
  eta: '',
});
const logFilePath = ref('');
const activeProfile = ref('');

// Path history state
const recentOutputPaths = ref([]);
const showOutputPathDropdown = ref(false);
let dropdownCloseTimeout = null;

// Computed properties
const canStartEncoding = computed(() => {
  return (
    selectedFiles.value.length > 0 &&
    outputDirectory.value &&
    activeProfile.value &&
    !isEncoding.value
  );
});

const statusText = computed(() => {
  if (isEncoding.value) {
    return 'Encoding in progress...';
  } else if (selectedFiles.value.length === 0) {
    return 'Select files to encode';
  } else if (!outputDirectory.value) {
    return 'Select output directory';
  } else if (!activeProfile.value) {
    return 'Select encoding profile';
  } else {
    return 'Ready to encode';
  }
});

const statusClass = computed(() => {
  if (isEncoding.value) return 'status-encoding';
  if (canStartEncoding.value) return 'status-ready';
  return 'status-waiting';
});

const statusIcon = computed(() => {
  if (isEncoding.value) return 'pi pi-spin pi-spinner';
  if (canStartEncoding.value) return 'pi pi-check-circle';
  return 'pi pi-info-circle';
});

// Methods
const getFileName = (filePath) => {
  return filePath.split(/[\\/]/).pop();
};

const selectFiles = async () => {
  try {
    isLoadingFiles.value = true;
    const result = await window.ipcRenderer.invoke('select-files');
    if (result.canceled) return;
    selectedFiles.value = result.filePaths;

    toast.add({
      severity: 'success',
      summary: 'Files Selected',
      detail: `${result.filePaths.length} file(s) selected for encoding`,
      life: 3000,
    });
  } catch (error) {
    console.error('Failed to select files:', error);
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: 'Failed to select files',
      life: 5000,
    });
  } finally {
    isLoadingFiles.value = false;
  }
};

const selectOutputDirectory = async () => {
  try {
    isLoadingOutput.value = true;
    const result = await window.ipcRenderer.invoke('select-output-directory');
    if (result.canceled) return;
    outputDirectory.value = result.filePaths[0];

    // Refresh the list of recent paths
    await loadRecentOutputPaths();

    toast.add({
      severity: 'success',
      summary: 'Output Directory Set',
      detail: 'Output directory selected successfully',
      life: 3000,
    });
  } catch (error) {
    console.error('Failed to select output directory:', error);
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: 'Failed to select output directory',
      life: 5000,
    });
  } finally {
    isLoadingOutput.value = false;
  }
};

const clearAll = () => {
  selectedFiles.value = [];
  outputDirectory.value = '';
  logFilePath.value = '';

  toast.add({
    severity: 'info',
    summary: 'Cleared',
    detail: 'All selections have been cleared',
    life: 2000,
  });
};

const copyOutputPathToClipboard = async () => {
  try {
    await navigator.clipboard.writeText(outputDirectory.value);
    toast.add({
      severity: 'success',
      summary: 'Copied',
      detail: 'Output path copied to clipboard',
      life: 2000,
    });
  } catch (error) {
    console.error('Failed to copy to clipboard:', error);
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: 'Failed to copy to clipboard',
      life: 3000,
    });
  }
};

const startEncoding = async () => {
  try {
    isStarting.value = true;

    // Debug logging
    console.log('Starting encoding process with:', {
      fileCount: selectedFiles.value.length,
      outputDirectory: outputDirectory.value,
      profileName: activeProfile.value,
    });

    // Ensure we're passing only serializable data
    const filePaths = [...selectedFiles.value];
    const outDir = String(outputDirectory.value);
    const profile = String(activeProfile.value);

    // Fetch the profile first to check if we need to use multi-resolution mode
    const profiles = await window.ipcRenderer.invoke('get-encoding-profiles');
    const selectedProfile = profiles[profile];

    // Determine if we need to use parallel encoding or regular encoding
    if (selectedProfile && selectedProfile.multiResolution) {
      console.log('Multi-resolution encoding enabled. Using parallel encoder');

      // For multi-resolution encoding, we process each file individually with parallel resolutions
      for (const filePath of filePaths) {
        const result = await window.ipcRenderer.invoke(
          'start-parallel-encoding',
          {
            file: filePath,
            outputDirectory: outDir,
            profileName: profile,
          }
        );

        if (!result.success) {
          console.error('Failed to start parallel encoding:', result.error);
          toast.add({
            severity: 'error',
            summary: 'Encoding Failed',
            detail: `Failed to start parallel encoding: ${result.error}`,
            life: 5000,
          });
          isEncoding.value = false;
          return;
        }
      }
    } else {
      // Use the standard encoding for single resolution
      const result = await window.ipcRenderer.invoke('start-encoding', {
        files: filePaths,
        outputDirectory: outDir,
        profileName: profile,
      });

      if (!result.success) {
        console.error('Failed to start encoding:', result.error);
        toast.add({
          severity: 'error',
          summary: 'Encoding Failed',
          detail: `Failed to start encoding: ${result.error}`,
          life: 5000,
        });
        isEncoding.value = false;
        return;
      }
    }

    console.log('Encoding started successfully, beginning progress monitoring');
    isEncoding.value = true;

    toast.add({
      severity: 'success',
      summary: 'Encoding Started',
      detail: `Started encoding ${filePaths.length} file(s)`,
      life: 3000,
    });

    // Start progress monitoring
    startProgressMonitoring();
  } catch (error) {
    console.error('Exception during encoding start:', error);
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: `Error: ${error.message || 'Unknown error occurred'}`,
      life: 5000,
    });
    isEncoding.value = false;
  } finally {
    isStarting.value = false;
  }
};

const stopEncoding = async () => {
  try {
    await window.ipcRenderer.invoke('stop-encoding');
    isEncoding.value = false;

    toast.add({
      severity: 'info',
      summary: 'Encoding Stopped',
      detail: 'Encoding process has been stopped',
      life: 3000,
    });
  } catch (error) {
    console.error('Failed to stop encoding:', error);
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: 'Failed to stop encoding',
      life: 3000,
    });
  }
};

const viewLogs = () => {
  if (logFilePath.value) {
    // Open log file in default text editor or show in file explorer
    window.ipcRenderer.invoke('open-log-file', logFilePath.value);
  }
};

const copyLogPathToClipboard = async () => {
  try {
    await navigator.clipboard.writeText(logFilePath.value);
    toast.add({
      severity: 'success',
      summary: 'Copied',
      detail: 'Log file path copied to clipboard',
      life: 2000,
    });
  } catch (error) {
    console.error('Failed to copy to clipboard:', error);
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: 'Failed to copy to clipboard',
      life: 3000,
    });
  }
};

const updateActiveProfile = (profile) => {
  activeProfile.value = profile;
};

const startProgressMonitoring = () => {
  // Clear any existing interval
  if (progressInterval) {
    clearInterval(progressInterval);
  }

  progressInterval = setInterval(async () => {
    if (!isEncoding.value) {
      clearInterval(progressInterval);
      return;
    }

    try {
      const progress = await window.ipcRenderer.invoke('get-encoding-progress');
      encodingProgress.value = progress;

      // Log progress changes only when significant changes occur
      if (Math.floor(progress.percent) % 5 === 0) {
        console.log('Encoding progress:', {
          percent: progress.percent.toFixed(1),
          speed: progress.speed,
          eta: progress.eta,
        });
      }

      if (progress.completed) {
        console.log('Encoding process completed');
        isEncoding.value = false;
        clearInterval(progressInterval);

        toast.add({
          severity: 'success',
          summary: 'Encoding Complete',
          detail: 'All files have been successfully encoded',
          life: 5000,
        });
      }
    } catch (error) {
      console.error('Failed to get encoding progress:', error);
    }
  }, 1000);
};

// Load recent output paths from the main process
const loadRecentOutputPaths = async () => {
  try {
    recentOutputPaths.value = await window.ipcRenderer.invoke(
      'get-recent-output-paths'
    );
  } catch (error) {
    console.error('Failed to get recent output paths:', error);
  }
};

// Load the most recent output path
const loadMostRecentOutputPath = async () => {
  try {
    const path = await window.ipcRenderer.invoke('get-most-recent-output-path');
    if (path) {
      outputDirectory.value = path;
    }
  } catch (error) {
    console.error('Failed to get most recent output path:', error);
  }
};

// Methods for log file handling
const loadLogFilePath = async () => {
  try {
    const path = await window.ipcRenderer.invoke('get-log-file-path');
    logFilePath.value = path;
  } catch (error) {
    console.error('Failed to get log file path:', error);
  }
};

// Lifecycle hooks
let progressInterval = null;

onMounted(async () => {
  console.log('EncoderMain component mounted');

  // Load recent paths
  await loadRecentOutputPaths();

  // Auto-fill with most recent path
  await loadMostRecentOutputPath();

  // Load log file path
  await loadLogFilePath();

  // Listen for encoding completion
  window.ipcRenderer.on('encoding-completed', () => {
    console.log('Received encoding-completed event');
    isEncoding.value = false;
  });

  window.ipcRenderer.on('encoding-error', (event, error) => {
    console.error('Received encoding-error event:', error);
    toast.add({
      severity: 'error',
      summary: 'Encoding Error',
      detail: `Encoding error: ${error}`,
      life: 5000,
    });
    isEncoding.value = false;
  });

  // Add listener for the new encoding-stopped event
  window.ipcRenderer.on('encoding-stopped', () => {
    console.log('Received encoding-stopped event');
    isEncoding.value = false;
  });
});

onUnmounted(() => {
  if (progressInterval) {
    clearInterval(progressInterval);
  }

  if (dropdownCloseTimeout) {
    clearTimeout(dropdownCloseTimeout);
  }

  // Remove event listeners
  window.ipcRenderer.off('encoding-completed');
  window.ipcRenderer.off('encoding-error');
  window.ipcRenderer.off('encoding-stopped');
});
</script>

<style scoped>
/* Additional mobile optimizations */
@media (max-width: 480px) {
  .encoder-actions {
    flex-direction: column;
    gap: 0.5rem;
  }

  .action-btn {
    width: 100%;
    justify-content: center;
  }

  .status-indicator {
    padding: 0.75rem 1rem;
    font-size: 0.9rem;
  }

  .status-indicator i {
    font-size: 1.1rem;
  }

  .files-list {
    gap: 0.5rem;
  }

  .file-item {
    padding: 0.5rem 0.75rem;
  }

  .file-item span {
    font-size: 0.8rem;
  }

  .output-path {
    padding: 0.75rem;
  }

  .output-path span {
    font-size: 0.8rem;
  }
}

/* Enhanced loading states */
.select-btn.p-button-loading {
  position: relative;
}

.select-btn.p-button-loading::after {
  content: '';
  position: absolute;
  width: 20px;
  height: 20px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  right: 1rem;
  top: 50%;
  transform: translateY(-50%);
}

/* Enhanced hover effects for interactive elements */
.file-item:hover {
  transform: translateX(4px);
}

.output-path:hover {
  transform: translateX(4px);
}

/* Custom scroll styling for long file lists */
.files-list {
  max-height: 300px;
  overflow-y: auto;
}

.files-list::-webkit-scrollbar {
  width: 6px;
}

.files-list::-webkit-scrollbar-track {
  background: var(--bg-tertiary);
  border-radius: 3px;
}

.files-list::-webkit-scrollbar-thumb {
  background: var(--border-color);
  border-radius: 3px;
}

.files-list::-webkit-scrollbar-thumb:hover {
  background: var(--border-hover);
}

/* Enhanced focus states for accessibility */
.action-btn:focus-visible {
  outline: 2px solid var(--primary-color);
  outline-offset: 2px;
}

.select-btn:focus-visible {
  outline: 2px solid var(--primary-color);
  outline-offset: 2px;
}

/* Progress bar enhancements */
.encoding-progress .p-progressbar .p-progressbar-value {
  background: linear-gradient(
    90deg,
    var(--primary-color),
    var(--primary-light)
  );
  border-radius: 6px;
  transition: width 0.3s ease;
}

.encoding-progress .p-progressbar {
  background: var(--bg-tertiary);
  border-radius: 6px;
}

/* Tag enhancements */
.files-count .p-tag {
  font-weight: 600;
  padding: 0.5rem 1rem;
  border-radius: 8px;
}

.active-profile .p-tag {
  font-weight: 600;
  padding: 0.5rem 1rem;
  border-radius: 8px;
}

/* Button icon spacing improvements */
.action-btn .p-button-icon {
  margin-right: 0.5rem;
}

.select-btn .p-button-icon {
  margin-right: 0.5rem;
}

/* Enhanced card transitions */
.selection-card,
.encoder-card,
.profiles-card {
  will-change: transform, box-shadow;
}

/* Status indicator pulse animation for encoding state */
.status-encoding {
  animation: pulse 2s ease-in-out infinite;
}

@keyframes pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.8;
  }
}

/* Enhanced tooltip positioning */
.p-tooltip {
  font-size: 0.875rem;
  max-width: 250px;
  word-wrap: break-word;
}

/* Improved input group styling */
.log-input-group .p-button {
  border-top-left-radius: 0;
  border-bottom-left-radius: 0;
}

.log-input-group .p-inputtext {
  border-top-right-radius: 0;
  border-bottom-right-radius: 0;
  border-right: none;
}

.log-input-group .p-inputtext:focus {
  border-right: 1px solid var(--primary-color) !important;
}

/* Native Desktop Encoder Styles */
.native-encoder-container {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  height: 100%;
  overflow-y: auto;
}

.native-card {
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 6px;
  overflow: hidden;
}

.native-card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.native-card-header .header-icon {
  color: var(--primary-color);
  font-size: 14px;
}

.native-card-header h3 {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
}

/* File selection content */
.file-selection-content {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.selected-files-info {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.files-count {
  display: flex;
  align-items: center;
}

.files-tag {
  font-size: 11px;
  padding: 2px 6px;
}

.files-preview {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.file-preview-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  background: var(--bg-secondary);
  border-radius: 3px;
  font-size: 12px;
  color: var(--text-secondary);
}

.file-preview-item i {
  color: var(--text-muted);
  font-size: 10px;
}

.more-files-indicator {
  display: flex;
  align-items: center;
  padding: 4px 8px;
  font-size: 11px;
  color: var(--text-muted);
  font-style: italic;
}

/* Output directory content */
.output-directory-content {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.directory-input-group {
  display: flex;
  gap: 4px;
}

.directory-input {
  flex: 1;
  font-size: 12px;
  padding: 4px 8px;
}

.directory-info {
  margin-top: 8px;
}

.directory-path {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  background: var(--bg-secondary);
  border-radius: 3px;
  font-size: 12px;
  color: var(--text-secondary);
}

.directory-path i {
  color: var(--text-muted);
  font-size: 10px;
}

/* Responsive adjustments for native desktop */
@media (max-width: 768px) {
  .native-encoder-container {
    padding: 12px;
    gap: 12px;
  }

  .file-preview-item,
  .directory-path {
    font-size: 11px;
  }
}
</style>
