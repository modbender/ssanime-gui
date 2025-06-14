<template>
  <div class="native-content">
    <div class="profiles-container">
      <!-- Profile Header -->
      <div class="profiles-header">
        <div class="profile-info">
          <h3>Encoding Profiles</h3>
          <span class="profile-description"
            >Manage your encoding presets and configurations</span
          >
        </div>
        <div class="profile-actions">
          <Button
            label="Save Changes"
            icon="pi pi-save"
            @click="saveProfile"
            severity="success"
            size="small"
          />
          <Button
            label="Reset to Default"
            icon="pi pi-refresh"
            severity="info"
            size="small"
            outlined
            @click="resetToDefault"
          />
          <Button
            v-if="!isDefaultProfile(activeProfile)"
            label="Delete Profile"
            icon="pi pi-trash"
            severity="danger"
            size="small"
            outlined
            @click="deleteProfile"
          />
        </div>
      </div>

      <!-- Profile Selection -->
      <div class="profile-selection">
        <div class="selection-group">
          <label for="profile-select">Select Profile</label>
          <div class="selection-controls">
            <Dropdown
              id="profile-select"
              v-model="activeProfile"
              :options="profileOptions"
              optionLabel="label"
              optionValue="value"
              placeholder="Select encoding profile..."
              @change="handleProfileChange"
              style="flex: 1"
            />
            <Button
              icon="pi pi-plus"
              @click="createNewProfile"
              severity="info"
              outlined
              v-tooltip="'Create New Profile'"
            />
          </div>
        </div>
      </div>

      <!-- Settings Sections -->
      <div class="settings-container">
        <!-- Video Settings -->
        <div class="settings-section">
          <div class="section-header">
            <i class="pi pi-video"></i>
            <h4>Video Settings</h4>
          </div>
          <div class="settings-grid">
            <div class="setting-item">
              <label for="crf-slider">CRF (Quality)</label>
              <div class="slider-container">
                <Slider
                  id="crf-slider"
                  v-model="currentSettings.crf"
                  :min="0"
                  :max="51"
                  :step="1"
                />
                <span class="slider-value">{{ currentSettings.crf }}</span>
              </div>
              <small class="setting-hint"
                >Lower values = higher quality, larger files</small
              >
            </div>

            <div class="setting-item">
              <label for="resolution-select">Resolution</label>
              <Dropdown
                id="resolution-select"
                v-model="currentSettings.resolution"
                :options="resolutionOptions"
                optionLabel="label"
                optionValue="value"
                placeholder="Select resolution..."
              />
            </div>

            <div class="setting-item">
              <label for="format-select">Format</label>
              <Dropdown
                id="format-select"
                v-model="currentSettings.format"
                :options="formatOptions"
                optionLabel="label"
                optionValue="value"
                placeholder="Select format..."
              />
            </div>

            <div class="setting-item">
              <label for="deblock-input">Deblock</label>
              <InputText
                id="deblock-input"
                v-model="currentSettings.deblock"
                placeholder="e.g., -1:-1"
              />
              <small class="setting-hint">Deblocking filter strength</small>
            </div>
          </div>

          <!-- Checkbox Settings -->
          <div class="checkbox-grid">
            <div class="checkbox-item">
              <Checkbox
                id="smartblur-check"
                v-model="currentSettings.smartblur"
                binary
              />
              <label for="smartblur-check">Smart Blur</label>
            </div>

            <div class="checkbox-item">
              <Checkbox
                id="deinterlace-check"
                v-model="currentSettings.deinterlace"
                binary
              />
              <label for="deinterlace-check">Deinterlace</label>
            </div>

            <div class="checkbox-item">
              <Checkbox
                id="hardsubs-check"
                v-model="currentSettings.hardsubs"
                binary
              />
              <label for="hardsubs-check">Hard Subs</label>
            </div>
          </div>
        </div>

        <!-- Advanced Settings -->
        <div class="settings-section">
          <div class="section-header">
            <i class="pi pi-cog"></i>
            <h4>Advanced Settings</h4>
          </div>
          <div class="settings-grid">
            <div class="setting-item">
              <label for="psy-rd-slider">Psy-RD</label>
              <div class="slider-container">
                <Slider
                  id="psy-rd-slider"
                  v-model="currentSettings.psy_rd"
                  :min="0"
                  :max="5"
                  :step="0.1"
                />
                <span class="slider-value">{{
                  currentSettings.psy_rd.toFixed(1)
                }}</span>
              </div>
            </div>

            <div class="setting-item">
              <label for="psy-rdoq-slider">Psy-RDOQ</label>
              <div class="slider-container">
                <Slider
                  id="psy-rdoq-slider"
                  v-model="currentSettings.psy_rdoq"
                  :min="0"
                  :max="5"
                  :step="0.1"
                />
                <span class="slider-value">{{
                  currentSettings.psy_rdoq.toFixed(1)
                }}</span>
              </div>
            </div>

            <div class="setting-item">
              <label for="aq-mode-select">AQ Mode</label>
              <Dropdown
                id="aq-mode-select"
                v-model="currentSettings.aq_mode"
                :options="aqModeOptions"
                optionLabel="label"
                optionValue="value"
                placeholder="Select AQ mode..."
              />
            </div>

            <div class="setting-item">
              <label for="aq-strength-slider">AQ Strength</label>
              <div class="slider-container">
                <Slider
                  id="aq-strength-slider"
                  v-model="currentSettings.aq_strength"
                  :min="0"
                  :max="3"
                  :step="0.1"
                />
                <span class="slider-value">{{
                  currentSettings.aq_strength.toFixed(1)
                }}</span>
              </div>
            </div>

            <div class="setting-item">
              <label for="me-input">Motion Estimation</label>
              <InputText
                id="me-input"
                v-model="currentSettings.me"
                placeholder="e.g., 2"
              />
            </div>

            <div class="setting-item">
              <label for="rd-input">Rate Distortion</label>
              <InputText
                id="rd-input"
                v-model="currentSettings.rd"
                placeholder="e.g., 4"
              />
            </div>

            <div class="setting-item">
              <label for="subme-input">SubME</label>
              <InputText
                id="subme-input"
                v-model="currentSettings.subme"
                placeholder="e.g., 7"
              />
            </div>

            <div class="setting-item">
              <label for="merange-input">ME Range</label>
              <InputText
                id="merange-input"
                v-model="currentSettings.merange"
                placeholder="e.g., 57"
              />
            </div>

            <div class="setting-item">
              <label for="b-frames-slider">B Frames</label>
              <div class="slider-container">
                <Slider
                  id="b-frames-slider"
                  v-model="currentSettings.b_frames"
                  :min="0"
                  :max="16"
                  :step="1"
                />
                <span class="slider-value">{{ currentSettings.b_frames }}</span>
              </div>
            </div>

            <div class="setting-item">
              <label for="b-adapt-slider">B Adapt</label>
              <div class="slider-container">
                <Slider
                  id="b-adapt-slider"
                  v-model="currentSettings.b_adapt"
                  :min="0"
                  :max="2"
                  :step="1"
                />
                <span class="slider-value">{{ currentSettings.b_adapt }}</span>
              </div>
            </div>

            <div class="setting-item">
              <label for="limit-sao-check">Limit SAO</label>
              <Checkbox
                id="limit-sao-check"
                v-model="currentSettings.limit_sao"
                binary
              />
            </div>

            <div class="setting-item">
              <label for="frame-threads-input">Frame Threads</label>
              <InputText
                id="frame-threads-input"
                v-model="currentSettings.frame_threads"
                placeholder="e.g., 4"
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
// No manual imports needed - Nuxt 3 auto-imports

// Profiles state - loaded from backend
const profiles = ref({});
const activeProfile = ref('SSAnime MKV');
const currentSettings = reactive({});
const isCustomProfile = ref(false);
const originalSettings = ref({});
const defaultProfiles = ['SSAnime MKV', 'SSAnime MP4'];

// Load profiles from backend
onMounted(async () => {
  try {
    const loadedProfiles = await window.ipcRenderer.invoke(
      'get-encoding-profiles'
    );
    profiles.value = loadedProfiles;

    // Set first available profile as active
    const profileKeys = Object.keys(loadedProfiles);
    if (profileKeys.length > 0) {
      activeProfile.value = profileKeys[0];
      Object.assign(currentSettings, loadedProfiles[profileKeys[0]]);
      originalSettings.value = { ...loadedProfiles[profileKeys[0]] };
    }
  } catch (error) {
    console.error('Error loading profiles:', error);
  }
});

// Current state

// Options for dropdowns
const profileOptions = computed(() => {
  const options = Object.keys(profiles.value).map((key) => ({
    label: key,
    value: key,
  }));

  // Add Custom option if current profile is custom
  if (isCustomProfile.value) {
    options.push({ label: 'Custom', value: 'Custom' });
  }

  return options;
});

const resolutionOptions = [
  { label: 'Original', value: 'original' },
  { label: '4K (3840x2160)', value: 3840 },
  { label: '1080p (1920x1080)', value: 1080 },
  { label: '720p (1280x720)', value: 720 },
  { label: '480p (854x480)', value: 480 },
];

const formatOptions = [
  { label: 'MKV', value: 'mkv' },
  { label: 'MP4', value: 'mp4' },
  { label: 'AVI', value: 'avi' },
  { label: 'MOV', value: 'mov' },
];

const aqModeOptions = [
  { label: 'None', value: 0 },
  { label: 'Variance AQ', value: 1 },
  { label: 'Auto-variance AQ', value: 2 },
  { label: 'Auto-variance AQ with bias', value: 3 },
];

// Methods
const handleProfileChange = () => {
  if (activeProfile.value === 'Custom') return;

  if (profiles.value[activeProfile.value]) {
    Object.assign(currentSettings, profiles.value[activeProfile.value]);
    originalSettings.value = { ...profiles.value[activeProfile.value] };
    isCustomProfile.value = false;
  }
};

const checkForChanges = () => {
  if (defaultProfiles.includes(activeProfile.value)) {
    const hasChanges = Object.keys(currentSettings).some(
      (key) => currentSettings[key] !== originalSettings.value[key]
    );

    if (hasChanges && !isCustomProfile.value) {
      isCustomProfile.value = true;
      activeProfile.value = 'Custom';
    }
  }
};

const saveProfile = async () => {
  try {
    if (isCustomProfile.value) {
      // Prompt for custom profile name
      const profileName = prompt('Enter a name for this custom profile:');
      if (!profileName) return;

      // Check if name already exists
      if (profiles.value[profileName]) {
        alert(
          'A profile with this name already exists. Please choose a different name.'
        );
        return;
      }

      // Save as new profile
      profiles.value[profileName] = { ...currentSettings };
      activeProfile.value = profileName;
      isCustomProfile.value = false;
    } else {
      // Update existing profile (only if not default)
      if (!defaultProfiles.includes(activeProfile.value)) {
        profiles.value[activeProfile.value] = { ...currentSettings };
      }
    }

    // Save to backend
    await window.ipcRenderer.invoke('save-encoding-profiles', profiles.value);
    originalSettings.value = { ...currentSettings };

    console.log('Profile saved:', activeProfile.value, currentSettings);
    alert('Profile saved successfully!');
  } catch (error) {
    console.error('Failed to save profile:', error);
    alert('Failed to save profile');
  }
};

const resetToDefault = () => {
  if (profiles.value[activeProfile.value]) {
    Object.assign(currentSettings, profiles.value[activeProfile.value]);
    originalSettings.value = { ...profiles.value[activeProfile.value] };
    isCustomProfile.value = false;
  }
};

const deleteProfile = async () => {
  if (isDefaultProfile(activeProfile.value)) return;

  delete profiles.value[activeProfile.value];

  // Switch to first available profile
  const profileKeys = Object.keys(profiles.value);
  if (profileKeys.length > 0) {
    activeProfile.value = profileKeys[0];
    Object.assign(currentSettings, profiles.value[profileKeys[0]]);
    originalSettings.value = { ...profiles.value[profileKeys[0]] };
  }

  try {
    await window.ipcRenderer.invoke('save-encoding-profiles', profiles.value);
    console.log('Profile deleted');
    alert('Profile deleted successfully!');
  } catch (error) {
    console.error('Failed to delete profile:', error);
    alert('Failed to delete profile');
  }
};

const createNewProfile = () => {
  const name = prompt('Enter profile name:');
  if (name && !profiles[name]) {
    profiles[name] = { ...currentSettings };
    activeProfile.value = name;
    alert('Profile created successfully!');
  }
};

const isDefaultProfile = (profileName) => {
  return defaultProfiles.includes(profileName);
};

// Watch for changes in settings to detect custom modifications
watch(
  () => ({ ...currentSettings }),
  () => {
    checkForChanges();
  },
  { deep: true }
);

// Watch for profile changes
watch(activeProfile, () => {
  handleProfileChange();
});
</script>

<style scoped>
.native-content {
  height: 100%;
  padding: 20px;
  overflow-y: auto;
}

.profiles-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.profiles-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  padding: 16px;
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 8px;
}

.profile-info h3 {
  margin: 0 0 4px 0;
  color: var(--text-primary);
  font-size: 18px;
  font-weight: 600;
}

.profile-description {
  color: var(--text-secondary);
  font-size: 14px;
}

.profile-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.profile-selection {
  padding: 16px;
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 8px;
}

.selection-group label {
  display: block;
  margin-bottom: 8px;
  color: var(--text-primary);
  font-weight: 500;
}

.selection-controls {
  display: flex;
  gap: 8px;
  align-items: center;
}

.settings-container {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.settings-section {
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

.section-header h4 {
  margin: 0;
  color: var(--text-primary);
  font-size: 16px;
  font-weight: 600;
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 16px;
  margin-bottom: 16px;
}

.setting-item {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.setting-item label {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.setting-hint {
  font-size: 11px;
  color: var(--text-secondary);
  font-style: italic;
}

.slider-container {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
}

.slider-container .p-slider {
  flex: 1;
  min-width: 100px;
}

.slider-value {
  min-width: 40px;
  text-align: center;
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--bg-secondary);
  padding: 4px 8px;
  border-radius: 4px;
}

.checkbox-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
}

.checkbox-item {
  display: flex;
  align-items: center;
  gap: 8px;
}

.checkbox-item label {
  color: var(--text-primary);
  font-size: 13px;
  cursor: pointer;
}
</style>
