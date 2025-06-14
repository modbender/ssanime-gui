<template>
  <div class="native-content">
    <div class="encoder-container">
      <!-- File Selection Section -->
      <div class="encoder-section">
        <div class="section-header">
          <i class="pi pi-file-video"></i>
          <h3>Input Files</h3>
        </div>
        <div class="section-content">
          <div class="file-selection">
            <Button
              label="Select Files"
              icon="pi pi-folder-open"
              @click="selectFiles"
              :loading="isLoadingFiles"
              severity="info"
              size="small"
            />
            <div v-if="selectedFiles.length > 0" class="selected-files">
              <div class="files-header">
                <Tag
                  :value="`${selectedFiles.length} files selected`"
                  severity="success"
                />
                <Button
                  :label="isFilesExpanded ? 'Collapse' : 'Expand'"
                  :icon="
                    isFilesExpanded ? 'pi pi-chevron-up' : 'pi pi-chevron-down'
                  "
                  @click="isFilesExpanded = !isFilesExpanded"
                  text
                  size="small"
                />
                <Button
                  label="Clear All"
                  icon="pi pi-trash"
                  @click="clearAllFiles"
                  severity="danger"
                  text
                  size="small"
                />
              </div>

              <div v-if="isFilesExpanded" class="files-list">
                <div
                  v-for="(file, index) in selectedFiles"
                  :key="index"
                  class="file-item"
                >
                  <i class="pi pi-file"></i>
                  <span class="file-name">{{ getFileName(file) }}</span>
                  <Button
                    icon="pi pi-times"
                    @click="removeFile(index)"
                    severity="danger"
                    text
                    size="small"
                    class="remove-btn"
                  />
                </div>
              </div>

              <div v-else class="files-preview">
                <div
                  v-for="(file, index) in selectedFiles.slice(0, 3)"
                  :key="index"
                  class="file-item"
                >
                  <i class="pi pi-file"></i>
                  <span>{{ getFileName(file) }}</span>
                </div>
                <div v-if="selectedFiles.length > 3" class="more-files">
                  <span>+{{ selectedFiles.length - 3 }} more files</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Output Directory Section -->
      <div class="encoder-section">
        <div class="section-header">
          <i class="pi pi-folder"></i>
          <h3>Output Directory</h3>
        </div>
        <div class="section-content">
          <div class="output-selection">
            <div class="output-path">
              <InputText
                v-model="outputDirectory"
                placeholder="Select output directory..."
                readonly
                style="flex: 1"
              />
              <Button
                icon="pi pi-folder-open"
                @click="selectOutputDirectory"
                severity="secondary"
                outlined
              />
            </div>
          </div>
        </div>
      </div>

      <!-- Encoding Profile Section -->
      <div class="encoder-section">
        <div class="section-header">
          <i class="pi pi-cog"></i>
          <h3>Encoding Profile</h3>
        </div>
        <div class="section-content">
          <div class="profile-selection">
            <Dropdown
              v-model="selectedProfile"
              :options="encodingProfiles"
              optionLabel="name"
              optionValue="id"
              placeholder="Select encoding profile..."
              style="width: 100%; max-width: 300px"
            />
            <Button
              label="Manage Profiles"
              icon="pi pi-wrench"
              severity="secondary"
              outlined
              size="small"
              @click="navigateToProfiles"
            />
          </div>
        </div>
      </div>

      <!-- Quick Settings Section -->
      <div class="encoder-section">
        <div class="section-header">
          <i class="pi pi-sliders-h"></i>
          <h3>Quick Settings</h3>
        </div>
        <div class="section-content">
          <div class="quick-settings">
            <div class="setting-group">
              <label>Quality</label>
              <Slider
                v-model="quickSettings.quality"
                :min="0"
                :max="100"
                :step="1"
              />
              <span>{{ quickSettings.quality }}%</span>
            </div>

            <div class="setting-group">
              <label>Resolution</label>
              <Dropdown
                v-model="quickSettings.resolution"
                :options="resolutionOptions"
                optionLabel="label"
                optionValue="value"
                placeholder="Original"
              />
            </div>

            <div class="setting-group">
              <label>Format</label>
              <Dropdown
                v-model="quickSettings.format"
                :options="formatOptions"
                optionLabel="label"
                optionValue="value"
                placeholder="MP4"
              />
            </div>
          </div>
        </div>
      </div>

      <!-- Action Buttons -->
      <div class="encoder-actions">
        <Button
          label="Add to Queue"
          icon="pi pi-plus"
          @click="addToQueue"
          :disabled="!canEncode"
          severity="success"
        />
        <Button
          label="Start Encoding"
          icon="pi pi-play"
          @click="startEncoding"
          :disabled="!canEncode"
          severity="info"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
// No manual imports needed - Nuxt 3 auto-imports Vue and composables

// Define emits
const $emit = defineEmits(['navigate-to-profiles']);

// State
const selectedFiles = ref([]);
const outputDirectory = ref('');
const selectedProfile = ref(null);
const isLoadingFiles = ref(false);
const isFilesExpanded = ref(false);

// Quick settings
const quickSettings = ref({
  quality: 80,
  resolution: 'original',
  format: 'mp4',
});

// Encoding profiles will be loaded from the profiles service
const encodingProfiles = ref([]);

// Load encoding profiles on component mount
onMounted(async () => {
  try {
    const profiles = await window.ipcRenderer.invoke('get-encoding-profiles');
    if (profiles && typeof profiles === 'object') {
      encodingProfiles.value = Object.keys(profiles).map((key) => ({
        id: key,
        name: profiles[key].name || key,
      }));

      // Set default profile if available
      if (encodingProfiles.value.length > 0) {
        selectedProfile.value = encodingProfiles.value[0].id;
      }
    } else {
      console.warn('No encoding profiles received');
    }
  } catch (error) {
    console.error('Error loading encoding profiles:', error);
    // Add a fallback profile
    encodingProfiles.value = [{ id: 'default', name: 'Default Profile' }];
    selectedProfile.value = 'default';
  }
});

// Resolution options
const resolutionOptions = [
  { label: 'Original', value: 'original' },
  { label: '4K (3840x2160)', value: '3840x2160' },
  { label: '1080p (1920x1080)', value: '1920x1080' },
  { label: '720p (1280x720)', value: '1280x720' },
  { label: '480p (854x480)', value: '854x480' },
];

// Format options
const formatOptions = [
  { label: 'MP4', value: 'mp4' },
  { label: 'MKV', value: 'mkv' },
  { label: 'AVI', value: 'avi' },
  { label: 'MOV', value: 'mov' },
];

// Computed
const canEncode = computed(() => {
  return (
    selectedFiles.value.length > 0 &&
    outputDirectory.value &&
    selectedProfile.value
  );
});

// Methods
const selectFiles = async () => {
  isLoadingFiles.value = true;
  try {
    const result = await window.ipcRenderer.invoke('select-files');
    if (!result.canceled && result.filePaths) {
      selectedFiles.value = result.filePaths;
    }
  } catch (error) {
    console.error('Error selecting files:', error);
  } finally {
    isLoadingFiles.value = false;
  }
};

const selectOutputDirectory = async () => {
  try {
    const result = await window.ipcRenderer.invoke('select-output-directory');
    if (!result.canceled && result.filePaths && result.filePaths.length > 0) {
      outputDirectory.value = result.filePaths[0];
    }
  } catch (error) {
    console.error('Error selecting output directory:', error);
  }
};

const getFileName = (filePath) => {
  return filePath.split('/').pop() || filePath.split('\\').pop() || filePath;
};

const removeFile = (index) => {
  selectedFiles.value.splice(index, 1);
  // If no files left, collapse the list
  if (selectedFiles.value.length === 0) {
    isFilesExpanded.value = false;
  }
};

const clearAllFiles = () => {
  selectedFiles.value = [];
  isFilesExpanded.value = false;
};

const navigateToProfiles = () => {
  // This will be handled by the parent component
  $emit('navigate-to-profiles');
};

const addToQueue = () => {
  // TODO: Add to encoding queue service
  console.log('Adding to queue:', {
    files: selectedFiles.value,
    output: outputDirectory.value,
    profile: selectedProfile.value,
    settings: quickSettings.value,
  });
};

const startEncoding = () => {
  // TODO: Start encoding service
  console.log('Starting encoding:', {
    files: selectedFiles.value,
    output: outputDirectory.value,
    profile: selectedProfile.value,
    settings: quickSettings.value,
  });
};
</script>

<style scoped>
.native-content {
  height: 100%;
  padding: 20px;
  overflow-y: auto;
}

.encoder-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.encoder-section {
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  padding: 16px;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--border-color);
}

.section-header i {
  color: var(--text-secondary);
  font-size: 16px;
}

.section-header h3 {
  margin: 0;
  color: var(--text-primary);
  font-size: 16px;
  font-weight: 600;
}

.section-content {
  color: var(--text-primary);
}

.file-selection {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.selected-files {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.files-header {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.files-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 200px;
  overflow-y: auto;
  border: 1px solid var(--border-color);
  border-radius: 4px;
  padding: 8px;
}

.files-preview {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.file-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  background: var(--bg-secondary);
  border-radius: 4px;
  font-size: 13px;
}

.file-item .file-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-item .remove-btn {
  opacity: 0;
  transition: opacity 0.2s ease;
}

.file-item:hover .remove-btn {
  opacity: 1;
}

.file-item i {
  color: var(--text-secondary);
}

.more-files {
  padding: 4px 8px;
  font-size: 12px;
  color: var(--text-secondary);
  font-style: italic;
}

.output-selection {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.output-path {
  display: flex;
  gap: 8px;
  align-items: center;
}

.profile-selection {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
}

.quick-settings {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 20px;
}

.setting-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.setting-group label {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.setting-group span {
  font-size: 12px;
  color: var(--text-secondary);
  text-align: center;
}

.encoder-actions {
  display: flex;
  gap: 12px;
  justify-content: center;
  padding: 16px;
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  margin-top: auto;
}
</style>
