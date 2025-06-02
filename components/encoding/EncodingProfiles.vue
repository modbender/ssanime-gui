<template>
  <Card class="profiles-card">
    <template #content>
      <div class="profiles-content">
        <div class="profiles-header">
          <h3 class="profiles-title">Encoding Profiles</h3>
          <div class="profiles-actions">
            <Button
              label="Save Changes"
              icon="pi pi-save"
              @click="saveProfile"
              severity="success"
            />
            <Button
              label="Reset to Default"
              icon="pi pi-refresh"
              severity="info"
              @click="resetToDefault"
            />
            <Button
              v-if="!isDefaultProfile(activeProfile)"
              label="Delete Profile"
              icon="pi pi-trash"
              severity="danger"
              @click="deleteProfile"
            />
          </div>
        </div>
        <Divider />
        <div class="profiles-select">
          <Select
            v-model="activeProfile"
            :options="Object.keys(profiles)"
            placeholder="Select Profile"
            @change="handleProfileChange"
            class="profile-dropdown"
          />
          <Button
            icon="pi pi-plus"
            @click="createNewProfile"
            severity="info"
            class="add-profile-btn"
          />
        </div>
        <Divider />
        <div class="settings-section">
          <h4 class="section-title">Video Settings</h4>
          <div class="p-fluid grid formgrid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label>CRF (Quality)</label>
              <Slider v-model="currentSettings.crf" :min="0" :max="51" />
            </div>
            <div>
              <label>Resolution</label>
              <Select
                v-model="currentSettings.resolution"
                :options="[480, 720, 1080]"
              />
            </div>
            <div>
              <label>Format</label>
              <Select
                v-model="currentSettings.format"
                :options="['mkv', 'mp4']"
              />
            </div>
            <div>
              <label>Deblock</label>
              <InputText v-model="currentSettings.deblock" />
            </div>
            <div>
              <label>Smart Blur</label>
              <Checkbox v-model="currentSettings.smartblur" />
            </div>
            <div>
              <label>Deinterlace</label>
              <Checkbox v-model="currentSettings.deinterlace" />
            </div>
            <div>
              <label>Hard Subs</label>
              <Checkbox v-model="currentSettings.hardsubs" />
            </div>
          </div>
        </div>
        <Divider />
        <div class="settings-section">
          <h4 class="section-title">Advanced Settings</h4>
          <div class="p-fluid grid formgrid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label>Psy-RD</label>
              <Slider
                v-model="currentSettings.psy_rd"
                :min="0"
                :max="5"
                :step="0.1"
              />
            </div>
            <div>
              <label>Psy-RDOQ</label>
              <Slider
                v-model="currentSettings.psy_rdoq"
                :min="0"
                :max="5"
                :step="0.1"
              />
            </div>
            <div>
              <label>AQ Strength</label>
              <Slider
                v-model="currentSettings.aq_strength"
                :min="0"
                :max="3"
                :step="0.1"
              />
            </div>
            <div>
              <label>AQ Mode</label>
              <InputText v-model="currentSettings.aq_mode" />
            </div>
            <div>
              <label>ME</label>
              <InputText v-model="currentSettings.me" />
            </div>
            <div>
              <label>RD</label>
              <InputText v-model="currentSettings.rd" />
            </div>
            <div>
              <label>SubME</label>
              <InputText v-model="currentSettings.subme" />
            </div>
            <div>
              <label>ME Range</label>
              <InputText v-model="currentSettings.merange" />
            </div>
            <div>
              <label>B-Frames</label>
              <InputText v-model="currentSettings.bframes" />
            </div>
            <div>
              <label>B-Adapt</label>
              <InputText v-model="currentSettings.b_adapt" />
            </div>
            <div>
              <label>Limit SAO</label>
              <Checkbox v-model="currentSettings.limit_sao" />
            </div>
            <div>
              <label>Frame Threads</label>
              <InputText v-model="currentSettings.frame_threads" />
            </div>
          </div>
        </div>
      </div>
    </template>
  </Card>
</template>

<script setup>
const emit = defineEmits(['profile-changed']);

// Declare 'profiles' only once
const profiles = reactive({
  'SSAnime MKV': {
    // Video settings
    crf: 23,
    deblock: '0:0',
    smartblur: false,
    deinterlace: false,
    resolution: 720,
    psy_rd: 1.0,
    psy_rdoq: 1.0,
    aq_strength: 1.0,
    hardsubs: false,
    multiResolution: true,
    outputResolutions: [720],
    me: 2,
    rd: 4,
    subme: 7,
    aq_mode: 3,
    merange: 57,
    bframes: 8,
    b_adapt: 2,
    limit_sao: true,
    frame_threads: 3,
    format: 'mkv',
  },
  'SSAnime MP4': {
    crf: 23,
    deblock: '0:0',
    smartblur: false,
    deinterlace: false,
    resolution: 720,
    psy_rd: 1.0,
    psy_rdoq: 1.0,
    aq_strength: 1.0,
    hardsubs: true,
    multiResolution: true,
    outputResolutions: [720],
    me: 2,
    rd: 4,
    subme: 7,
    aq_mode: 3,
    merange: 57,
    bframes: 8,
    b_adapt: 2,
    limit_sao: true,
    frame_threads: 3,
    format: 'mp4',
  },
});

const activeProfile = ref('');
const currentSettings = reactive({
  // Video settings
  crf: 23,
  deblock: '0:0',
  smartblur: false,
  deinterlace: false,
  resolution: 720,
  psy_rd: 1.0,
  psy_rdoq: 1.0,
  aq_strength: 1.0,
  hardsubs: false,
  multiResolution: true, // Set to true by default
  outputResolutions: [720],

  // Advanced x265 params
  me: 2,
  rd: 4,
  subme: 7,
  aq_mode: 3,
  merange: 57,
  bframes: 8,
  b_adapt: 2,
  limit_sao: true,
  frame_threads: 3,

  // Output format
  format: 'mkv',
});

// Computed properties to convert between single values and arrays for sliders
const crfValue = computed({
  get: () => [currentSettings.crf],
  set: (val) => {
    currentSettings.crf = val[0];
  },
});

const aqStrengthValue = computed({
  get: () => [currentSettings.aq_strength],
  set: (val) => {
    currentSettings.aq_strength = val[0];
  },
});

const psyRdValue = computed({
  get: () => [currentSettings.psy_rd],
  set: (val) => {
    currentSettings.psy_rd = val[0];
  },
});

const psyRdoqValue = computed({
  get: () => [currentSettings.psy_rdoq],
  set: (val) => {
    currentSettings.psy_rdoq = val[0];
  },
});

const resolutionSelections = reactive({
  480: false,
  720: true,
  1080: false,
});

// Update outputResolutions array when checkbox selections change
const updateOutputResolutions = () => {
  const selectedResolutions = Object.entries(resolutionSelections)
    .filter(([_, isSelected]) => isSelected)
    .map(([resolution, _]) => parseInt(resolution, 10));

  if (selectedResolutions.length === 0) {
    // Ensure at least one resolution is selected
    resolutionSelections[720] = true;
    currentSettings.outputResolutions = [720];
  } else {
    currentSettings.outputResolutions = selectedResolutions;
  }

  console.log('Selected resolutions:', currentSettings.outputResolutions);
};

// Watch for multi-resolution toggle
watch(
  () => currentSettings.multiResolution,
  (isMulti) => {
    if (isMulti) {
      // When enabling multi-resolution, initialize with current resolution
      Object.keys(resolutionSelections).forEach((res) => {
        resolutionSelections[res] =
          parseInt(res, 10) === currentSettings.resolution;
      });
      updateOutputResolutions();
    }
  }
);

// Check if profile is one of the default profiles
const isDefaultProfile = (profileName) => {
  return ['SSAnime MKV', 'SSAnime MP4'].includes(profileName);
};

// Check if this property is protected by the profile type
const isProfileTypeProtected = (property) => {
  if (!isDefaultProfile(activeProfile.value)) return false;

  if (
    activeProfile.value === 'SSAnime MKV' &&
    (property === 'format' || property === 'hardsubs')
  ) {
    return true;
  }

  if (
    activeProfile.value === 'SSAnime MP4' &&
    (property === 'format' || property === 'hardsubs')
  ) {
    return true;
  }

  return false;
};

// Load profiles from localStorage/electron-store on component mount
onMounted(async () => {
  try {
    const savedProfiles = await window.ipcRenderer.invoke(
      'get-encoding-profiles'
    );
    if (savedProfiles && Object.keys(savedProfiles).length > 0) {
      Object.assign(profiles, savedProfiles);
    }
  } catch (error) {
    console.error('Failed to load encoding profiles:', error);
  }

  // Initialize with the first default profile
  handleProfileChange(activeProfile.value);

  // Emit initial active profile
  emit('profile-changed', activeProfile.value);
});

// Watch for active profile changes and emit event
watch(activeProfile, (newProfile) => {
  emit('profile-changed', newProfile);
});

const handleProfileChange = (profile) => {
  activeProfile.value = profile;
  Object.assign(currentSettings, profiles[profile]);

  // Force a reactivity update on the computed slider values
  // This ensures the sliders update when changing profiles
  crfValue.value = [currentSettings.crf];
  aqStrengthValue.value = [currentSettings.aq_strength];
  psyRdValue.value = [currentSettings.psy_rd];
  psyRdoqValue.value = [currentSettings.psy_rdoq];
};

const saveProfile = async () => {
  // If it's a default profile and we're saving, preserve the required values
  if (isDefaultProfile(activeProfile.value)) {
    if (activeProfile.value === 'SSAnime MKV') {
      currentSettings.format = 'mkv';
      currentSettings.hardsubs = false;
    } else if (activeProfile.value === 'SSAnime MP4') {
      currentSettings.format = 'mp4';
      currentSettings.hardsubs = true;
    }
  }

  profiles[activeProfile.value] = { ...currentSettings };

  try {
    await window.ipcRenderer.invoke('save-encoding-profiles', profiles);
    emit('profile-changed', activeProfile.value);
  } catch (error) {
    console.error('Failed to save encoding profiles:', error);
  }
};

const createNewProfile = () => {
  const profileName = `Profile ${Object.keys(profiles).length + 1}`;
  profiles[profileName] = { ...currentSettings };
  activeProfile.value = profileName;
  saveProfile();
};

const resetToDefault = () => {
  if (activeProfile.value === 'SSAnime MKV') {
    Object.assign(currentSettings, {
      ...profiles['SSAnime MKV'],
      hardsubs: false,
      format: 'mkv',
    });
  } else if (activeProfile.value === 'SSAnime MP4') {
    Object.assign(currentSettings, {
      ...profiles['SSAnime MP4'],
      hardsubs: true,
      format: 'mp4',
    });
  } else {
    // For custom profiles, reset to base settings
    const baseSettings = {
      // Video settings
      crf: 23,
      deblock: '0:0',
      smartblur: false,
      deinterlace: false,
      resolution: 720,
      psy_rd: 1.0,
      psy_rdoq: 1.0,
      aq_strength: 1.0,
      hardsubs: false,
      multiResolution: false,
      outputResolutions: [720],

      // Advanced x265 params
      me: 2,
      rd: 4,
      subme: 7,
      aq_mode: 3,
      merange: 57,
      bframes: 8,
      b_adapt: 2,
      limit_sao: true,
      frame_threads: 3,

      // Output format
      format: 'mkv',
    };

    Object.assign(currentSettings, baseSettings);
  }

  // Refresh computed slider values
  crfValue.value = [currentSettings.crf];
  aqStrengthValue.value = [currentSettings.aq_strength];
  psyRdValue.value = [currentSettings.psy_rd];
  psyRdoqValue.value = [currentSettings.psy_rdoq];
};

const deleteProfile = async () => {
  if (isDefaultProfile(activeProfile.value)) return;

  delete profiles[activeProfile.value];
  activeProfile.value = 'SSAnime MKV';
  Object.assign(currentSettings, profiles['SSAnime MKV']);

  try {
    await window.ipcRenderer.invoke('save-encoding-profiles', profiles);
  } catch (error) {
    console.error('Failed to delete profile:', error);
  }
};
</script>

<style scoped>
.profiles-card {
  background: #23272f;
  border-radius: 14px;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.1);
  margin-bottom: 24px;
}
.profiles-content {
  padding: 24px 16px 16px 16px;
}
.profiles-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}
.profiles-title {
  font-size: 1.3rem;
  font-weight: 700;
  color: #fff;
}
.profiles-actions {
  display: flex;
  gap: 8px;
}
.profiles-select {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.profile-dropdown {
  min-width: 180px;
}
.add-profile-btn {
  min-width: 40px;
}
.settings-section {
  margin-bottom: 16px;
}
.section-title {
  font-size: 1.1rem;
  font-weight: 600;
  color: #b0b6c3;
  margin-bottom: 8px;
}
</style>
