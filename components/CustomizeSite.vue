<template>
  <Dialog>
    <DialogTrigger as-child>
      <Button variant="ghost" size="icon">
        <Icon name="tabler:brush" />
      </Button>
    </DialogTrigger>
    <DialogContent class="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>Customize Interface</DialogTitle>
        <DialogDescription>
          Change the theme and appearance of the application.
        </DialogDescription>
      </DialogHeader>

      <Separator />

      <div>
        <Label class="block mb-3">Color</Label>
        <ToggleGroup
          type="single"
          class="flex-wrap"
          :modelValue="color"
          @update:modelValue="handleColorChange"
        >
          <ToggleGroupItem
            v-for="colorName in colorList"
            :value="colorName"
            :key="colorName"
          >
            <div class="mr-2" :class="colorName">
              <div class="w-3 h-3 bg-primary"></div>
            </div>
            {{ useCapitalize(colorName) }}
          </ToggleGroupItem>
        </ToggleGroup>
      </div>

      <Separator />

      <div>
        <Label class="block mb-3">Border Radius</Label>
        <ToggleGroup
          type="single"
          class="flex-wrap"
          :modelValue="radius"
          @update:modelValue="handleRadiusChange"
        >
          <ToggleGroupItem
            v-for="radiusItem in radiusList"
            :value="radiusItem.value"
            :key="radiusItem.value"
          >
            {{ radiusItem.name }}
          </ToggleGroupItem>
        </ToggleGroup>
      </div>

      <Separator />

      <div>
        <Label class="block mb-3">Theme</Label>
        <ToggleGroup
          type="single"
          class="flex-wrap"
          :modelValue="$colorMode.preference"
          @update:modelValue="handleThemeChange"
        >
          <ToggleGroupItem
            v-for="themeItem in themeList"
            :value="themeItem.value"
            :key="themeItem.value"
            class="gap-2 items-center"
          >
            <Icon :name="themeItem.icon" />
            {{ useCapitalize(themeItem.value) }}
          </ToggleGroupItem>
        </ToggleGroup>
      </div>
    </DialogContent>
  </Dialog>
</template>

<script setup>
import { storeToRefs } from 'pinia';
import { useThemeStore } from '~/stores/theme';
import { useColorMode } from '#imports';
import { useCapitalize } from '~/composables/capitalize';

const colorMode = useColorMode();
const themeStore = useThemeStore();

const { setColor, setRadius } = themeStore;

const { color, radius } = storeToRefs(themeStore);

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
    icon: 'tabler:devices',
  },
  {
    value: 'light',
    icon: 'tabler:sun',
  },
  {
    value: 'dark',
    icon: 'tabler:moon',
  },
];

const handleColorChange = (value) => {
  if (!value) {
    value = colorList[0];
  }
  setColor(value);
};

const handleRadiusChange = (value) => {
  if (!value) {
    value = radiusList[2].value;
  }
  setRadius(value);
};

const handleThemeChange = (value) => {
  if (!value) {
    value = themeList[0].value;
  }

  colorMode.preference = value;
};
</script>

<style scoped>
button[data-state='on'] {
  border-width: 2px;
  border-color: hsl(var(--primary));
}
</style>
