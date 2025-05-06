<template>
  <Card class="w-full">
    <CardHeader>
      <CardTitle class="text-xl font-bold">Encoding Profiles</CardTitle>
    </CardHeader>

    <CardContent class="p-6">
      <!-- Profile selection tabs -->
      <div class="mb-4 border-b border-border">
        <ul class="flex flex-wrap -mb-px">
          <li
            v-for="profile in Object.keys(profiles)"
            :key="profile"
            class="mr-2"
          >
            <Button
              variant="ghost"
              :class="
                activeProfile === profile
                  ? 'border-b-2 border-primary rounded-b-none'
                  : 'text-muted-foreground'
              "
              @click="handleProfileChange(profile)"
            >
              {{ profile }}
            </Button>
          </li>
          <li>
            <Button variant="ghost" size="icon" @click="createNewProfile">
              <Icon name="tabler:plus" class="h-5 w-5" />
            </Button>
          </li>
        </ul>
      </div>

      <!-- Action buttons -->
      <div class="flex flex-wrap gap-2 mb-6">
        <Button @click="saveProfile" variant="default"> Save Changes </Button>
        <Button @click="resetToDefault" variant="outline">
          Reset to Default
        </Button>
        <Button
          v-if="!isDefaultProfile(activeProfile)"
          @click="deleteProfile"
          variant="destructive"
        >
          Delete Profile
        </Button>
      </div>

      <!-- Profile settings -->
      <div class="space-y-6">
        <!-- Video Settings Card -->
        <Card>
          <CardHeader>
            <CardTitle class="text-lg">Video Settings</CardTitle>
          </CardHeader>
          <CardContent class="grid grid-cols-1 md:grid-cols-2 gap-6">
            <!-- CRF (Quality) -->
            <div class="col-span-full">
              <div class="flex justify-between mb-2">
                <Label for="crf-slider">CRF (Quality)</Label>
                <Badge variant="outline">{{
                  Number(currentSettings.crf)
                }}</Badge>
              </div>
              <div class="flex flex-col space-y-2">
                <Slider
                  id="crf-slider"
                  :modelValue="crfValue"
                  @update:modelValue="(val) => (crfValue = val)"
                  :min="0"
                  :max="51"
                  :step="1"
                  class="w-full"
                />
                <div class="flex justify-between text-xs text-muted-foreground">
                  <span>0 (Lossless)</span>
                  <span>23 (Default)</span>
                  <span>51 (Worst)</span>
                </div>
              </div>
            </div>

            <!-- Resolution Selection -->
            <div class="col-span-full">
              <Label class="mb-3 block">Output Resolutions</Label>
              <div class="flex flex-wrap gap-4 bg-muted/20 p-4 rounded-md">
                <div class="flex items-center space-x-2">
                  <Checkbox
                    id="res480"
                    v-model:checked="resolutionSelections[480]"
                    @change="updateOutputResolutions"
                  />
                  <Label for="res480">480p</Label>
                </div>

                <div class="flex items-center space-x-2">
                  <Checkbox
                    id="res720"
                    v-model:checked="resolutionSelections[720]"
                    @change="updateOutputResolutions"
                  />
                  <Label for="res720">720p</Label>
                </div>

                <div class="flex items-center space-x-2">
                  <Checkbox
                    id="res1080"
                    v-model:checked="resolutionSelections[1080]"
                    @change="updateOutputResolutions"
                  />
                  <Label for="res1080">1080p</Label>
                </div>
              </div>
              <span class="text-xs text-muted-foreground mt-1 block">
                Upscaling is automatically disabled
              </span>
            </div>

            <!-- Deblock -->
            <div>
              <Label for="deblock">Deblock</Label>
              <Input
                id="deblock"
                v-model="currentSettings.deblock"
                class="mt-1"
              />
            </div>

            <!-- AQ Strength -->
            <div>
              <div class="flex justify-between mb-2">
                <Label for="aq-slider">AQ Strength</Label>
                <Badge variant="outline">{{
                  Number(currentSettings.aq_strength).toFixed(1)
                }}</Badge>
              </div>
              <div class="flex flex-col space-y-2">
                <Slider
                  id="aq-slider"
                  :modelValue="aqStrengthValue"
                  @update:modelValue="(val) => (aqStrengthValue = val)"
                  :min="0"
                  :max="3"
                  :step="0.1"
                  class="w-full"
                />
                <div class="flex justify-between text-xs text-muted-foreground">
                  <span>0 (Off)</span>
                  <span>1.0 (Default)</span>
                  <span>3.0 (Max)</span>
                </div>
              </div>
            </div>

            <!-- Psy-RD -->
            <div>
              <div class="flex justify-between mb-2">
                <Label for="psy-rd-slider">Psy-RD</Label>
                <Badge variant="outline">{{
                  Number(currentSettings.psy_rd).toFixed(1)
                }}</Badge>
              </div>
              <div class="flex flex-col space-y-2">
                <Slider
                  id="psy-rd-slider"
                  :modelValue="psyRdValue"
                  @update:modelValue="(val) => (psyRdValue = val)"
                  :min="0"
                  :max="5"
                  :step="0.1"
                  class="w-full"
                />
                <div class="flex justify-between text-xs text-muted-foreground">
                  <span>0 (Off)</span>
                  <span>1.0 (Default)</span>
                  <span>5.0 (Max)</span>
                </div>
              </div>
            </div>

            <!-- Psy-RDOQ -->
            <div>
              <div class="flex justify-between mb-2">
                <Label for="psy-rdoq-slider">Psy-RDOQ</Label>
                <Badge variant="outline">{{
                  Number(currentSettings.psy_rdoq).toFixed(1)
                }}</Badge>
              </div>
              <div class="flex flex-col space-y-2">
                <Slider
                  id="psy-rdoq-slider"
                  :modelValue="psyRdoqValue"
                  @update:modelValue="(val) => (psyRdoqValue = val)"
                  :min="0"
                  :max="5"
                  :step="0.1"
                  class="w-full"
                />
                <div class="flex justify-between text-xs text-muted-foreground">
                  <span>0 (Off)</span>
                  <span>1.0 (Default)</span>
                  <span>5.0 (Max)</span>
                </div>
              </div>
            </div>

            <!-- Checkboxes Group -->
            <div class="space-y-3">
              <!-- Smart Blur -->
              <div class="flex items-center space-x-2">
                <Checkbox
                  id="smartblur"
                  v-model:checked="currentSettings.smartblur"
                />
                <Label for="smartblur">Smart Blur</Label>
              </div>

              <!-- Deinterlace -->
              <div class="flex items-center space-x-2">
                <Checkbox
                  id="deinterlace"
                  v-model:checked="currentSettings.deinterlace"
                />
                <Label for="deinterlace">Deinterlace</Label>
              </div>

              <!-- Hard Subtitles -->
              <div class="flex items-center space-x-2">
                <Checkbox
                  id="hardsubs"
                  v-model:checked="currentSettings.hardsubs"
                  :disabled="isProfileTypeProtected('hardsubs')"
                />
                <Label for="hardsubs" class="flex items-center gap-1">
                  Hard Subtitles
                  <span
                    v-if="isProfileTypeProtected('hardsubs')"
                    class="text-xs text-muted-foreground"
                  >
                    (Fixed by profile type)
                  </span>
                </Label>
              </div>
            </div>

            <!-- Format -->
            <div>
              <Label for="format-select">Format</Label>
              <Select
                id="format-select"
                v-model="currentSettings.format"
                :disabled="isProfileTypeProtected('format')"
                class="mt-1"
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select format" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="mkv">MKV</SelectItem>
                  <SelectItem value="mp4">MP4</SelectItem>
                </SelectContent>
              </Select>
              <span
                v-if="isProfileTypeProtected('format')"
                class="text-xs text-muted-foreground mt-1 block"
              >
                Format is fixed by profile type
              </span>
            </div>
          </CardContent>
        </Card>

        <Separator class="my-6" />

        <!-- Advanced Settings -->
        <Card>
          <CardHeader>
            <CardTitle class="text-lg">Advanced Settings</CardTitle>
          </CardHeader>
          <CardContent class="grid grid-cols-1 md:grid-cols-3 gap-6">
            <div>
              <Label for="me">Motion Estimation</Label>
              <Input
                id="me"
                type="number"
                v-model.number="currentSettings.me"
                class="mt-1"
              />
            </div>
            <div>
              <Label for="rd">Rate Distortion</Label>
              <Input
                id="rd"
                type="number"
                v-model.number="currentSettings.rd"
                class="mt-1"
              />
            </div>
            <div>
              <Label for="subme">Subpixel ME</Label>
              <Input
                id="subme"
                type="number"
                v-model.number="currentSettings.subme"
                class="mt-1"
              />
            </div>
            <div>
              <Label for="aq_mode">AQ Mode</Label>
              <Input
                id="aq_mode"
                type="number"
                v-model.number="currentSettings.aq_mode"
                class="mt-1"
              />
            </div>
            <div>
              <Label for="merange">ME Range</Label>
              <Input
                id="merange"
                type="number"
                v-model.number="currentSettings.merange"
                class="mt-1"
              />
            </div>
            <div>
              <Label for="bframes">B-Frames</Label>
              <Input
                id="bframes"
                type="number"
                v-model.number="currentSettings.bframes"
                class="mt-1"
              />
            </div>
            <div>
              <Label for="b_adapt">B-Adapt</Label>
              <Input
                id="b_adapt"
                type="number"
                v-model.number="currentSettings.b_adapt"
                class="mt-1"
              />
            </div>
            <div>
              <Label for="frame_threads">Frame Threads</Label>
              <Input
                id="frame_threads"
                type="number"
                v-model.number="currentSettings.frame_threads"
                class="mt-1"
              />
            </div>
            <div>
              <div class="flex items-center space-x-2 h-full">
                <Checkbox
                  id="limit_sao"
                  v-model:checked="currentSettings.limit_sao"
                />
                <Label for="limit_sao">Limit SAO</Label>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </CardContent>
  </Card>
</template>

<script setup>
import { ref, reactive, onMounted, watch, computed } from 'vue';

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

// Profile templates
const profiles = reactive({
  'SSAnime MKV': {
    ...currentSettings,
    hardsubs: false,
    format: 'mkv',
    // Make multi-resolution the default
    multiResolution: true,
    outputResolutions: [720],
  },
  'SSAnime MP4': {
    ...currentSettings,
    hardsubs: true,
    format: 'mp4',
    // Make multi-resolution the default
    multiResolution: true,
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
