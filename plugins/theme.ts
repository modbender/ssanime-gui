export default defineNuxtPlugin(() => {
  const themeStore = useThemeStore();

  const { color, radius } = storeToRefs(themeStore);

  const colorClassName = computed(() => {
    if (!color.value) {
      return 'zinc';
    }

    return color.value;
  });

  const radiusClassName = computed(() => {
    if (!radius.value) {
      return 'radius-none';
    }

    return radius.value;
  });

  useHead({
    htmlAttrs: {
      class: [colorClassName, radiusClassName],
    },
  });
});
