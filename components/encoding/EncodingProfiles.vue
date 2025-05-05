<template>
  <div class="w-full p-4 bg-white rounded-lg shadow">
    <h2 class="text-xl font-bold text-blue-600 mb-4">Encoding Profiles</h2>

    <!-- Profile selection tabs -->
    <div class="mb-4 border-b border-gray-200">
      <ul class="flex flex-wrap -mb-px">
        <li
          v-for="profile in Object.keys(profiles)"
          :key="profile"
          class="mr-2"
        >
          <button
            @click="handleProfileChange(profile)"
            class="inline-block py-2 px-4 rounded-t-lg"
            :class="
              activeProfile === profile
                ? 'text-blue-600 border-b-2 border-blue-600'
                : 'text-gray-500 hover:text-gray-700 hover:border-gray-300'
            "
          >
            {{ profile }}
          </button>
        </li>
        <li>
          <button
            @click="createNewProfile"
            class="inline-block py-2 px-4 text-gray-500 hover:text-gray-700 rounded-t-lg"
          >
            <span class="text-xl">+</span>
          </button>
        </li>
      </ul>
    </div>

    <!-- Action buttons -->
    <div class="flex flex-wrap gap-2 mb-6">
      <button
        @click="saveProfile"
        class="bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded"
      >
        Save Changes
      </button>
      <button
        @click="resetToDefault"
        class="bg-gray-200 hover:bg-gray-300 text-gray-700 py-2 px-4 rounded"
      >
        Reset to Default
      </button>
      <button
        v-if="!isDefaultProfile(activeProfile)"
        @click="deleteProfile"
        class="bg-red-600 hover:bg-red-700 text-white py-2 px-4 rounded"
      >
        Delete Profile
      </button>
    </div>

    <!-- Profile settings -->
    <div class="space-y-6">
      <div>
        <h3 class="text-lg font-medium text-gray-900 mb-3">Video Settings</h3>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
          <!-- CRF (Quality) -->
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              CRF (Quality): {{ currentSettings.crf }}
            </label>
            <input
              type="range"
              v-model.number="currentSettings.crf"
              min="0"
              max="51"
              step="1"
              class="w-full h-2 bg-gray-200 rounded-lg appearance-none cursor-pointer"
            />
            <div class="flex justify-between text-xs text-gray-500">
              <span>0 (Lossless)</span>
              <span>23 (Default)</span>
              <span>51 (Worst)</span>
            </div>
          </div>

          <!-- Resolution -->
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              Resolution
            </label>
            <select
              v-model="currentSettings.resolution"
              class="block w-full p-2 border border-gray-300 rounded-md"
              :disabled="currentSettings.multiResolution"
            >
              <option :value="480">480p</option>
              <option :value="720">720p</option>
              <option :value="1080">1080p</option>
            </select>
            <span
              v-if="currentSettings.multiResolution"
              class="text-xs text-gray-500"
            >
              Single resolution disabled when multi-resolution is active
            </span>
          </div>

          <!-- Multi-Resolution Encoding -->
          <div class="col-span-1 md:col-span-2">
            <div class="flex items-center mb-2">
              <input
                type="checkbox"
                id="multiResolution"
                v-model="currentSettings.multiResolution"
                class="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
              />
              <label
                for="multiResolution"
                class="ml-2 block text-sm text-gray-700"
              >
                Enable Multi-Resolution Encoding
              </label>
            </div>

            <div
              v-if="currentSettings.multiResolution"
              class="bg-gray-50 p-3 rounded border border-gray-200"
            >
              <p class="text-sm text-gray-600 mb-2">
                Select output resolutions (upscaling will be prevented)
              </p>

              <div class="flex flex-wrap gap-3">
                <div class="flex items-center">
                  <input
                    type="checkbox"
                    id="res480"
                    v-model="resolutionSelections[480]"
                    @change="updateOutputResolutions"
                    class="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
                  />
                  <label for="res480" class="ml-2 block text-sm text-gray-700"
                    >480p</label
                  >
                </div>

                <div class="flex items-center">
                  <input
                    type="checkbox"
                    id="res720"
                    v-model="resolutionSelections[720]"
                    @change="updateOutputResolutions"
                    class="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
                  />
                  <label for="res720" class="ml-2 block text-sm text-gray-700"
                    >720p</label
                  >
                </div>

                <div class="flex items-center">
                  <input
                    type="checkbox"
                    id="res1080"
                    v-model="resolutionSelections[1080]"
                    @change="updateOutputResolutions"
                    class="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
                  />
                  <label for="res1080" class="ml-2 block text-sm text-gray-700"
                    >1080p</label
                  >
                </div>
              </div>
            </div>
          </div>

          <!-- Deblock -->
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              Deblock
            </label>
            <input
              type="text"
              v-model="currentSettings.deblock"
              class="block w-full p-2 border border-gray-300 rounded-md"
            />
          </div>

          <!-- AQ Strength -->
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              AQ Strength: {{ currentSettings.aq_strength }}
            </label>
            <input
              type="range"
              v-model.number="currentSettings.aq_strength"
              min="0"
              max="3"
              step="0.1"
              class="w-full h-2 bg-gray-200 rounded-lg appearance-none cursor-pointer"
            />
          </div>

          <!-- Psy-RD -->
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              Psy-RD: {{ currentSettings.psy_rd }}
            </label>
            <input
              type="range"
              v-model.number="currentSettings.psy_rd"
              min="0"
              max="5"
              step="0.1"
              class="w-full h-2 bg-gray-200 rounded-lg appearance-none cursor-pointer"
            />
          </div>

          <!-- Psy-RDOQ -->
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              Psy-RDOQ: {{ currentSettings.psy_rdoq }}
            </label>
            <input
              type="range"
              v-model.number="currentSettings.psy_rdoq"
              min="0"
              max="5"
              step="0.1"
              class="w-full h-2 bg-gray-200 rounded-lg appearance-none cursor-pointer"
            />
          </div>

          <!-- Smart Blur -->
          <div class="flex items-center">
            <input
              type="checkbox"
              id="smartblur"
              v-model="currentSettings.smartblur"
              class="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
            />
            <label for="smartblur" class="ml-2 block text-sm text-gray-700">
              Smart Blur
            </label>
          </div>

          <!-- Deinterlace -->
          <div class="flex items-center">
            <input
              type="checkbox"
              id="deinterlace"
              v-model="currentSettings.deinterlace"
              class="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
            />
            <label for="deinterlace" class="ml-2 block text-sm text-gray-700">
              Deinterlace
            </label>
          </div>

          <!-- Hard Subtitles -->
          <div class="flex items-center">
            <input
              type="checkbox"
              id="hardsubs"
              v-model="currentSettings.hardsubs"
              :disabled="isProfileTypeProtected('hardsubs')"
              class="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
            />
            <label for="hardsubs" class="ml-2 block text-sm text-gray-700">
              Hard Subtitles
              <span
                v-if="isProfileTypeProtected('hardsubs')"
                class="text-xs text-gray-500"
              >
                (Fixed by profile type)
              </span>
            </label>
          </div>

          <!-- Format -->
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              Format
            </label>
            <select
              v-model="currentSettings.format"
              :disabled="isProfileTypeProtected('format')"
              class="block w-full p-2 border border-gray-300 rounded-md"
            >
              <option value="mkv">MKV</option>
              <option value="mp4">MP4</option>
            </select>
            <span
              v-if="isProfileTypeProtected('format')"
              class="text-xs text-gray-500"
            >
              Format is fixed by profile type
            </span>
          </div>
        </div>
      </div>

      <hr class="my-6" />

      <!-- Advanced Settings -->
      <div>
        <h3 class="text-lg font-medium text-gray-900 mb-3">
          Advanced Settings
        </h3>
        <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              Motion Estimation
            </label>
            <input
              type="number"
              v-model.number="currentSettings.me"
              class="block w-full p-2 border border-gray-300 rounded-md"
            />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              Rate Distortion
            </label>
            <input
              type="number"
              v-model.number="currentSettings.rd"
              class="block w-full p-2 border border-gray-300 rounded-md"
            />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              Subpixel ME
            </label>
            <input
              type="number"
              v-model.number="currentSettings.subme"
              class="block w-full p-2 border border-gray-300 rounded-md"
            />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              AQ Mode
            </label>
            <input
              type="number"
              v-model.number="currentSettings.aq_mode"
              class="block w-full p-2 border border-gray-300 rounded-md"
            />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              ME Range
            </label>
            <input
              type="number"
              v-model.number="currentSettings.merange"
              class="block w-full p-2 border border-gray-300 rounded-md"
            />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              B-Frames
            </label>
            <input
              type="number"
              v-model.number="currentSettings.bframes"
              class="block w-full p-2 border border-gray-300 rounded-md"
            />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              B-Adapt
            </label>
            <input
              type="number"
              v-model.number="currentSettings.b_adapt"
              class="block w-full p-2 border border-gray-300 rounded-md"
            />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              Frame Threads
            </label>
            <input
              type="number"
              v-model.number="currentSettings.frame_threads"
              class="block w-full p-2 border border-gray-300 rounded-md"
            />
          </div>
          <div class="flex items-center">
            <input
              type="checkbox"
              id="limit_sao"
              v-model="currentSettings.limit_sao"
              class="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
            />
            <label for="limit_sao" class="ml-2 block text-sm text-gray-700">
              Limit SAO
            </label>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, watch } from 'vue';

const emit = defineEmits(['profile-changed']);

const activeProfile = ref('SSAnime MKV');
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

  // Multi-resolution encoding
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

// Profile templates
const profiles = reactive({
  'SSAnime MKV': {
    ...currentSettings,
    hardsubs: false,
    format: 'mkv',
    multiResolution: false,
    outputResolutions: [720],
  },
  'SSAnime MP4': {
    ...currentSettings,
    hardsubs: true,
    format: 'mp4',
    multiResolution: false,
    outputResolutions: [720],
  },
});

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
