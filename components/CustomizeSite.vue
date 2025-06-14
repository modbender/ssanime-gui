<template>
  <Dialog v-model:visible="visible" modal header="Customize Interface">
    <div>
      <Divider />
      <div class="mb-3">
        <label>Color</label>
        <Select
          v-model="color"
          :options="colorList"
          placeholder="Select color"
        />
      </div>
      <Divider />
      <div class="mb-3">
        <label>Border Radius</label>
        <Select
          v-model="radius"
          :options="radiusList.map(r => r.value)"
          placeholder="Select radius"
        />
      </div>
      <Divider />
      <div class="mb-3">
        <label>Theme</label>
        <Select
          v-model="theme"
          :options="themeList.map(t => t.value)"
          placeholder="Select theme"
        />
      </div>
    </div>
    <template #footer>
      <Button label="Close" icon="pi pi-times" @click="visible = false" />
    </template>
  </Dialog>
  <Button
    icon="pi pi-brush"
    text
    rounded
    aria-label="Customize"
    @click="visible = true"
  />
</template>

<script setup>
// No manual imports needed - Nuxt 3 auto-imports Vue and composables

const visible = ref(false);
const color = ref('zinc');
const radius = ref('radius-md');
const theme = ref('system');
const colorList = [
  'zinc',
  'rose',
  'blue',
  'green',
  'orange',
  'red',
  'slate',
  'stone',
  'gray',
  'neutral',
  'yellow',
  'violet',
];
const radiusList = [
  {
    name: '0',
    value: 'radius-none',
  },
  {
    name: '0.25',
    value: 'radius-sm',
  },
  {
    name: '0.5',
    value: 'radius-md',
  },
  {
    name: '0.75',
    value: 'radius-lg',
  },
  {
    name: '1',
    value: 'radius-xl',
  },
];
const themeList = [
  {
    value: 'system',
    icon: 'pi pi-desktop',
  },
  {
    value: 'light',
    icon: 'pi pi-sun',
  },
  {
    value: 'dark',
    icon: 'pi pi-moon',
  },
];
</script>
