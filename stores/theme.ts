import { defineStore } from 'pinia';

type Color =
  | 'zinc'
  | 'rose'
  | 'blue'
  | 'green'
  | 'orange'
  | 'red'
  | 'slate'
  | 'stone'
  | 'gray'
  | 'neutral'
  | 'yellow'
  | 'violet';

type Radius =
  | 'radius-none'
  | 'radius-sm'
  | 'radius-md'
  | 'radius-lg'
  | 'radius-xl';

interface ThemeState {
  color: Color;
  radius: Radius;
}

export const useThemeStore = defineStore('theme', {
  state: (): ThemeState => {
    return {
      color: 'zinc',
      radius: 'radius-none',
    };
  },

  actions: {
    setColor(color: ThemeState['color']) {
      this.color = color;
    },

    setRadius(radius: ThemeState['radius']) {
      this.radius = radius;
    },
  },
});
