<template>
  <div class="p-4">
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
      <!-- File Input Section -->
      <div class="bg-white rounded-lg shadow p-4">
        <h2 class="text-xl font-bold text-blue-600 mb-4">Input Files</h2>

        <div class="mb-4">
          <button
            @click="selectFiles"
            :disabled="isEncoding"
            class="bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded flex items-center gap-2"
          >
            <Icon name="material-symbols:file-copy" />
            Select Files
          </button>
        </div>

        <div v-if="selectedFiles.length > 0">
          <h3 class="font-medium mb-2">
            Selected Files: {{ selectedFiles.length }}
          </h3>
          <div
            class="max-h-60 overflow-y-auto p-2 border border-gray-200 rounded"
          >
            <div
              v-for="(file, index) in selectedFiles"
              :key="index"
              class="mb-1 text-sm"
            >
              {{ getFileName(file) }}
            </div>
          </div>
        </div>
        <div v-else>
          <p class="text-gray-500">No files selected</p>
        </div>
      </div>

      <!-- Output Directory Section -->
      <div class="bg-white rounded-lg shadow p-4">
        <h2 class="text-xl font-bold text-blue-600 mb-4">Output Settings</h2>

        <div class="mb-4 relative">
          <label class="block text-sm font-medium text-gray-700 mb-1">
            Output Directory
          </label>
          <div class="flex">
            <div class="relative flex-1">
              <input
                type="text"
                v-model="outputDirectory"
                @click="showOutputPathDropdown = !showOutputPathDropdown"
                @blur="closeDropdownDelayed"
                class="block w-full p-2 border border-gray-300 rounded-l-md cursor-pointer"
                placeholder="Select output directory"
                readonly
              />

              <!-- Dropdown for recent paths -->
              <div
                v-if="showOutputPathDropdown && recentOutputPaths.length > 0"
                class="absolute z-10 w-full mt-1 bg-white border border-gray-300 rounded-md shadow-lg max-h-60 overflow-y-auto"
              >
                <div
                  v-for="(pathEntry, index) in recentOutputPaths"
                  :key="index"
                  @mousedown="selectExistingOutputPath(pathEntry.path)"
                  class="p-2 hover:bg-gray-100 cursor-pointer flex flex-col"
                >
                  <span class="text-sm font-medium">{{
                    pathEntry.label || getDirectoryName(pathEntry.path)
                  }}</span>
                  <span class="text-xs text-gray-500 truncate">{{
                    pathEntry.path
                  }}</span>
                  <span class="text-xs text-gray-400">{{
                    formatDate(pathEntry.lastUsed)
                  }}</span>
                </div>
                <div class="border-t border-gray-200">
                  <div
                    @mousedown="clearPathHistory"
                    class="p-2 hover:bg-gray-100 cursor-pointer text-sm text-red-600"
                  >
                    Clear History
                  </div>
                </div>
              </div>
            </div>
            <button
              @click="selectOutputDirectory"
              :disabled="isEncoding"
              class="bg-gray-200 hover:bg-gray-300 px-3 py-2 rounded-r-md flex items-center"
            >
              <Icon name="material-symbols:folder" />
            </button>
          </div>
        </div>

        <div class="mt-6">
          <h3 class="font-medium mb-2">Active Profile:</h3>
          <div class="p-2 bg-gray-100 rounded">
            {{ activeProfile }}
          </div>
        </div>
      </div>

      <!-- Controls -->
      <div class="bg-white rounded-lg shadow p-4">
        <h2 class="text-xl font-bold text-blue-600 mb-4">Encoder Controls</h2>

        <div class="flex flex-col gap-4">
          <button
            v-if="!isEncoding"
            @click="startEncoding"
            :disabled="!canStartEncoding"
            class="bg-green-600 hover:bg-green-700 text-white py-2 px-4 rounded flex items-center justify-center gap-2"
          >
            <Icon name="material-symbols:play-arrow" />
            Start Encoding
          </button>

          <button
            v-else
            @click="stopEncoding"
            class="bg-red-600 hover:bg-red-700 text-white py-2 px-4 rounded flex items-center justify-center gap-2"
          >
            <Icon name="material-symbols:stop" />
            Stop Encoding
          </button>
        </div>

        <div class="mt-6">
          <h3 class="font-medium mb-2">Status:</h3>
          <div class="p-2 rounded" :class="statusClass">
            {{ statusText }}
          </div>
        </div>

        <div v-if="errorMessage" class="error-message">
          {{ errorMessage }}
        </div>
      </div>
    </div>

    <!-- Encoding Profiles -->
    <div class="mt-6">
      <EncodingProfiles @profile-changed="updateActiveProfile" />
    </div>

    <!-- Progress Bar -->
    <EncodingProgress
      :visible="isEncoding"
      :progress="encodingProgress.percent"
      :current-file="encodingProgress.currentFile"
      :speed="encodingProgress.speed"
      :eta="encodingProgress.eta"
    />

    <!-- Log File Information -->
    <div class="mt-6 bg-white rounded-lg shadow p-4">
      <h2 class="text-xl font-bold text-blue-600 mb-2">Debug Information</h2>
      <div class="flex flex-col">
        <div class="mb-2">
          <h3 class="font-medium">Log File:</h3>
          <div class="flex items-center mt-1">
            <input
              type="text"
              v-model="logFilePath"
              readonly
              class="block w-full p-2 text-sm bg-gray-50 border border-gray-300 rounded-md"
            />
            <button
              @click="copyLogPathToClipboard"
              class="ml-2 bg-gray-200 hover:bg-gray-300 px-3 py-2 rounded-md flex items-center"
              title="Copy path to clipboard"
            >
              <Icon name="material-symbols:content-copy" />
            </button>
            <button
              @click="openLogFileLocation"
              class="ml-2 bg-gray-200 hover:bg-gray-300 px-3 py-2 rounded-md flex items-center"
              title="Open containing folder"
            >
              <Icon name="material-symbols:folder-open" />
            </button>
          </div>
          <p class="text-xs text-gray-500 mt-1">
            This log file can help with troubleshooting if you encounter any
            issues.
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue';
import EncodingProfiles from './EncodingProfiles.vue';
import EncodingProgress from './EncodingProgress.vue';

// State
const selectedFiles = ref([]);
const outputDirectory = ref('');
const isEncoding = ref(false);
const activeProfile = ref('SSAnime MKV');
const errorMessage = ref('');
const encodingProgress = ref({
  percent: 0,
  currentFile: '',
  speed: 'N/A',
  eta: 'Calculating...',
});
const logFilePath = ref('');

// Path history state
const recentOutputPaths = ref([]);
const showOutputPathDropdown = ref(false);
let dropdownCloseTimeout = null;

// Computed properties
const canStartEncoding = computed(() => {
  return (
    selectedFiles.value.length > 0 && outputDirectory.value && !isEncoding.value
  );
});

const statusText = computed(() => {
  if (isEncoding.value) {
    return 'Encoding in progress...';
  } else if (selectedFiles.value.length === 0) {
    return 'Select files to encode';
  } else if (!outputDirectory.value) {
    return 'Select output directory';
  } else {
    return 'Ready to encode';
  }
});

const statusClass = computed(() => {
  if (isEncoding.value) {
    return 'bg-yellow-100 text-yellow-800';
  } else if (canStartEncoding.value) {
    return 'bg-green-100 text-green-800';
  } else {
    return 'bg-gray-100 text-gray-800';
  }
});

// Methods
const selectFiles = async () => {
  try {
    const result = await window.ipcRenderer.invoke('select-files');
    if (result.canceled) return;
    selectedFiles.value = result.filePaths;
  } catch (error) {
    console.error('Failed to select files:', error);
  }
};

const selectOutputDirectory = async () => {
  try {
    const result = await window.ipcRenderer.invoke('select-output-directory');
    if (result.canceled) return;
    outputDirectory.value = result.filePaths[0];

    // Refresh the list of recent paths
    await loadRecentOutputPaths();
  } catch (error) {
    console.error('Failed to select output directory:', error);
  }
};

const startEncoding = async () => {
  try {
    // Clear any previous error messages
    errorMessage.value = '';

    // Debug logging
    console.log('Starting encoding process with:', {
      fileCount: selectedFiles.value.length,
      outputDirectory: outputDirectory.value,
      profileName: activeProfile.value,
    });

    isEncoding.value = true;

    // Ensure we're passing only serializable data
    // Convert any complex objects to simple strings/arrays/objects
    const filePaths = [...selectedFiles.value]; // Create a plain array copy
    const outDir = String(outputDirectory.value); // Ensure it's a string
    const profile = String(activeProfile.value); // Ensure it's a string

    console.log('Sending data to main process:', {
      fileCount: filePaths.length,
      outputDir: outDir,
      profile: profile,
    });

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
          errorMessage.value = `Failed to start parallel encoding: ${result.error}`;
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
        errorMessage.value = `Failed to start encoding: ${result.error}`;
        isEncoding.value = false;
        return;
      }
    }

    console.log('Encoding started successfully, beginning progress monitoring');
    // Start progress monitoring
    startProgressMonitoring();
  } catch (error) {
    console.error('Exception during encoding start:', error);
    errorMessage.value = `Error: ${error.message || 'Unknown error occurred'}`;
    isEncoding.value = false;
  }
};

const stopEncoding = async () => {
  try {
    await window.ipcRenderer.invoke('stop-encoding');
    isEncoding.value = false;
  } catch (error) {
    console.error('Failed to stop encoding:', error);
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
      }
    } catch (error) {
      console.error('Failed to get encoding progress:', error);
      errorMessage.value = `Failed to monitor encoding progress: ${error.message}`;
    }
  }, 1000);
};

const getFileName = (path) => {
  return path.split(/[\\/]/).pop();
};

// Helper function to format dates
const formatDate = (timestamp) => {
  if (!timestamp) return '';
  const date = new Date(timestamp);
  return date.toLocaleString();
};

// Helper function to get the directory name from a path
const getDirectoryName = (directoryPath) => {
  if (!directoryPath) return '';
  // Split by slashes (both forward and backward)
  const parts = directoryPath.split(/[\\/]/);
  return parts[parts.length - 1] || parts[parts.length - 2] || directoryPath;
};

// Select an existing output path from the dropdown
const selectExistingOutputPath = (path) => {
  outputDirectory.value = path;
  showOutputPathDropdown.value = false;
  if (dropdownCloseTimeout) {
    clearTimeout(dropdownCloseTimeout);
  }
};

// Close the dropdown after a small delay to allow clicks to register
const closeDropdownDelayed = () => {
  dropdownCloseTimeout = setTimeout(() => {
    showOutputPathDropdown.value = false;
  }, 200);
};

// Clear path history
const clearPathHistory = async () => {
  try {
    await window.ipcRenderer.invoke('clear-path-history');
    recentOutputPaths.value = []; // Clear local cache
    showOutputPathDropdown.value = false;
  } catch (error) {
    console.error('Failed to clear path history:', error);
  }
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

const copyLogPathToClipboard = () => {
  try {
    navigator.clipboard.writeText(logFilePath.value);
    console.log('Log file path copied to clipboard');
  } catch (error) {
    console.error('Failed to copy to clipboard:', error);
  }
};

const openLogFileLocation = async () => {
  try {
    const logDir = logFilePath.value.split(/[\\/]/).slice(0, -1).join('/');
    window.open(`file://${logDir}`, '_blank');
  } catch (error) {
    console.error('Failed to open log file location:', error);
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
    errorMessage.value = '';
  });

  window.ipcRenderer.on('encoding-error', (event, error) => {
    console.error('Received encoding-error event:', error);
    errorMessage.value = `Encoding error: ${error}`;
    isEncoding.value = false;
  });

  // Add listener for the new encoding-stopped event
  window.ipcRenderer.on('encoding-stopped', () => {
    console.log('Received encoding-stopped event');
    isEncoding.value = false;
    errorMessage.value = ''; // Don't show an error for manual stops
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
});
</script>

<style scoped>
.error-message {
  color: #d32f2f;
  background-color: #ffebee;
  padding: 8px 12px;
  border-radius: 4px;
  margin-top: 8px;
  font-size: 0.9rem;
}
</style>
