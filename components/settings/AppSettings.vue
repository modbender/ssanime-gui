<template>
  <div class="native-content">
    <div class="settings-container">
      <!-- Settings Navigation -->
      <div class="settings-nav">
        <div class="nav-title">Settings</div>
        <div class="nav-items">
          <button
            v-for="section in settingSections"
            :key="section.key"
            class="nav-item"
            :class="{ active: activeSection === section.key }"
            @click="activeSection = section.key"
          >
            <i :class="section.icon"></i>
            <span>{{ section.label }}</span>
          </button>
        </div>
      </div>

      <!-- Settings Content -->
      <div class="settings-content">
        <!-- General Settings -->
        <div v-if="activeSection === 'general'" class="settings-section">
          <h3>General Settings</h3>

          <div class="setting-group">
            <label class="setting-label">Application Theme</label>
            <div class="setting-control">
              <SelectButton
                v-model="settings.theme"
                :options="themeOptions"
                optionLabel="label"
                optionValue="value"
              />
            </div>
          </div>

          <div class="setting-group">
            <label class="setting-label">Language</label>
            <div class="setting-control">
              <Dropdown
                v-model="settings.language"
                :options="languageOptions"
                optionLabel="label"
                optionValue="value"
                style="width: 200px"
              />
            </div>
          </div>

          <div class="setting-group">
            <label class="setting-label">Auto-save Settings</label>
            <div class="setting-control">
              <ToggleSwitch v-model="settings.autoSave" />
            </div>
            <small class="setting-description">
              Automatically save settings when changed
            </small>
          </div>
        </div>

        <!-- Encoding Settings -->
        <div v-if="activeSection === 'encoding'" class="settings-section">
          <h3>Encoding Settings</h3>

          <div class="setting-group">
            <label class="setting-label">Default Output Directory</label>
            <div class="setting-control">
              <div class="path-input">
                <InputText
                  v-model="settings.outputDirectory"
                  placeholder="Select output directory..."
                  readonly
                  style="flex: 1"
                />
                <Button
                  icon="pi pi-folder-open"
                  severity="secondary"
                  @click="selectOutputDirectory"
                />
              </div>
            </div>
          </div>

          <div class="setting-group">
            <label class="setting-label">Maximum Concurrent Encodings</label>
            <div class="setting-control">
              <InputNumber
                v-model="settings.maxConcurrentEncodings"
                :min="1"
                :max="8"
                style="width: 100px"
              />
            </div>
            <small class="setting-description">
              Number of files that can be encoded simultaneously
            </small>
          </div>

          <div class="setting-group">
            <label class="setting-label"
              >Delete Source Files After Encoding</label
            >
            <div class="setting-control">
              <ToggleSwitch v-model="settings.deleteSourceFiles" />
            </div>
            <small class="setting-description">
              Automatically delete original files after successful encoding
            </small>
          </div>
        </div>

        <!-- Performance Settings -->
        <div v-if="activeSection === 'performance'" class="settings-section">
          <h3>Performance Settings</h3>

          <div class="setting-group">
            <label class="setting-label">Hardware Acceleration</label>
            <div class="setting-control">
              <Dropdown
                v-model="settings.hardwareAcceleration"
                :options="hwAccelOptions"
                optionLabel="label"
                optionValue="value"
                style="width: 200px"
              />
            </div>
          </div>

          <div class="setting-group">
            <label class="setting-label">CPU Priority</label>
            <div class="setting-control">
              <Dropdown
                v-model="settings.cpuPriority"
                :options="priorityOptions"
                optionLabel="label"
                optionValue="value"
                style="width: 150px"
              />
            </div>
          </div>

          <div class="setting-group">
            <label class="setting-label">Memory Limit (MB)</label>
            <div class="setting-control">
              <InputNumber
                v-model="settings.memoryLimit"
                :min="256"
                :max="8192"
                :step="256"
                style="width: 120px"
              />
            </div>
          </div>
        </div>

        <!-- Advanced Settings -->
        <div v-if="activeSection === 'advanced'" class="settings-section">
          <h3>Advanced Settings</h3>

          <div class="setting-group">
            <label class="setting-label">FFmpeg Path</label>
            <div class="setting-control">
              <div class="path-input">
                <InputText
                  v-model="settings.ffmpegPath"
                  placeholder="Auto-detect"
                  style="flex: 1"
                />
                <Button
                  icon="pi pi-folder-open"
                  severity="secondary"
                  @click="selectFFmpegPath"
                />
              </div>
            </div>
          </div>

          <div class="setting-group">
            <label class="setting-label">Enable Debug Logging</label>
            <div class="setting-control">
              <ToggleSwitch v-model="settings.debugLogging" />
            </div>
          </div>

          <div class="setting-group">
            <label class="setting-label">Log Retention (days)</label>
            <div class="setting-control">
              <InputNumber
                v-model="settings.logRetention"
                :min="1"
                :max="90"
                style="width: 100px"
              />
            </div>
          </div>
        </div>

        <!-- Settings Actions -->
        <div class="settings-actions">
          <Button
            label="Reset to Defaults"
            severity="danger"
            outlined
            @click="resetSettings"
          />
          <Button
            label="Export Settings"
            severity="info"
            outlined
            @click="exportSettings"
          />
          <Button
            label="Save Changes"
            severity="success"
            @click="saveSettings"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
// No manual imports needed - Nuxt 3 auto-imports Vue and composables

// Active section
const activeSection = ref('general');

// Setting sections
const settingSections = [
  { key: 'general', label: 'General', icon: 'pi pi-cog' },
  { key: 'encoding', label: 'Encoding', icon: 'pi pi-play-circle' },
  { key: 'performance', label: 'Performance', icon: 'pi pi-bolt' },
  { key: 'advanced', label: 'Advanced', icon: 'pi pi-wrench' },
];

// Settings data
const settings = reactive({
  // General
  theme: 'dark',
  language: 'en',
  autoSave: true,

  // Encoding
  outputDirectory: '',
  maxConcurrentEncodings: 2,
  deleteSourceFiles: false,

  // Performance
  hardwareAcceleration: 'auto',
  cpuPriority: 'normal',
  memoryLimit: 2048,

  // Advanced
  ffmpegPath: '',
  debugLogging: false,
  logRetention: 7,
});

// Options
const themeOptions = [
  { label: 'Light', value: 'light' },
  { label: 'Dark', value: 'dark' },
  { label: 'Auto', value: 'auto' },
];

const languageOptions = [
  { label: 'English', value: 'en' },
  { label: 'Spanish', value: 'es' },
  { label: 'French', value: 'fr' },
  { label: 'German', value: 'de' },
  { label: 'Japanese', value: 'ja' },
];

const hwAccelOptions = [
  { label: 'Auto Detect', value: 'auto' },
  { label: 'None', value: 'none' },
  { label: 'NVIDIA (NVENC)', value: 'nvenc' },
  { label: 'Intel (QSV)', value: 'qsv' },
  { label: 'AMD (AMF)', value: 'amf' },
];

const priorityOptions = [
  { label: 'Low', value: 'low' },
  { label: 'Normal', value: 'normal' },
  { label: 'High', value: 'high' },
];

// Methods
const selectOutputDirectory = () => {
  // In a real app, this would open a directory dialog
  console.log('Select output directory');
};

const selectFFmpegPath = () => {
  // In a real app, this would open a file dialog
  console.log('Select FFmpeg path');
};

const resetSettings = () => {
  // Reset all settings to defaults
  Object.assign(settings, {
    theme: 'dark',
    language: 'en',
    autoSave: true,
    outputDirectory: '',
    maxConcurrentEncodings: 2,
    deleteSourceFiles: false,
    hardwareAcceleration: 'auto',
    cpuPriority: 'normal',
    memoryLimit: 2048,
    ffmpegPath: '',
    debugLogging: false,
    logRetention: 7,
  });
};

const exportSettings = () => {
  // Export settings to JSON file
  console.log('Export settings');
};

const saveSettings = () => {
  // Save settings to configuration file
  console.log('Save settings');
};
</script>

<style scoped>
.native-content {
  height: 100%;
  overflow: hidden;
}

.settings-container {
  height: 100%;
  display: flex;
}

.settings-nav {
  width: 200px;
  background: var(--bg-secondary);
  border-right: 1px solid var(--border-color);
  flex-shrink: 0;
}

.nav-title {
  padding: 16px;
  font-weight: 600;
  color: var(--text-primary);
  border-bottom: 1px solid var(--border-color);
}

.nav-items {
  padding: 8px;
}

.nav-item {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border: none;
  background: transparent;
  color: var(--text-secondary);
  font-size: 13px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
  text-align: left;
  margin-bottom: 2px;
}

.nav-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.nav-item.active {
  background: var(--primary-color);
  color: white;
}

.nav-item i {
  font-size: 14px;
  width: 16px;
}

.settings-content {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
}

.settings-section {
  max-width: 600px;
}

.settings-section h3 {
  margin: 0 0 24px 0;
  color: var(--text-primary);
  font-size: 20px;
  font-weight: 600;
}

.setting-group {
  margin-bottom: 24px;
}

.setting-label {
  display: block;
  margin-bottom: 8px;
  font-weight: 500;
  color: var(--text-primary);
  font-size: 14px;
}

.setting-control {
  margin-bottom: 4px;
}

.setting-description {
  color: var(--text-secondary);
  font-size: 12px;
  line-height: 1.4;
}

.path-input {
  display: flex;
  gap: 8px;
  align-items: center;
}

.settings-actions {
  margin-top: 32px;
  padding-top: 24px;
  border-top: 1px solid var(--border-color);
  display: flex;
  gap: 12px;
}

/* Custom scrollbar */
.settings-content::-webkit-scrollbar {
  width: 8px;
}

.settings-content::-webkit-scrollbar-track {
  background: var(--bg-secondary);
}

.settings-content::-webkit-scrollbar-thumb {
  background: var(--border-color);
  border-radius: 4px;
}

.settings-content::-webkit-scrollbar-thumb:hover {
  background: var(--border-hover);
}
</style>
