import Aura from '@primeuix/themes/aura';

// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  devtools: { enabled: true },
  css: ['~/assets/scss/main.scss'],

  modules: [
    'nuxt-electron',
    '@nuxt/icon',
    '@pinia/nuxt',
    '@nuxtjs/color-mode',
    '@primevue/nuxt-module',
  ],

  // Disable SSR for Electron app (recommended by nuxt-electron)
  ssr: false,

  colorMode: {
    storage: 'cookie',
    classSuffix: '',
  },

  // Add baseURL config as recommended in nuxt-electron documentation
  app: {
    baseURL: './',
    buildAssetsDir: '/',
  },

  // Runtime config for baseURL
  runtimeConfig: {
    app: {
      baseURL: './',
      buildAssetsDir: '/',
    },
  },

  // Nitro config for baseURL
  nitro: {
    compatibilityDate: '2025-05-05',
    runtimeConfig: {
      app: {
        baseURL: './',
      },
    },
  },

  icon: {
    // Configure icon module for Electron environment
    size: '24px',
    class: 'icon',
    mode: 'svg',
  },

  electron: {
    build: [
      {
        // Main-Process entry file of the Electron App.
        entry: 'electron/main.ts',
      },
      {
        entry: 'electron/preload.ts',
        onstart(args: { reload: () => void }) {
          // Notify the Renderer-Process to reload the page when the Preload-Scripts build is complete,
          // instead of restarting the entire Electron App.
          args.reload();
        },
      },
    ],
    // Polyfill the Electron and Node.js API for Renderer process.
    renderer: {},
  },

  primevue: {
    options: {
      theme: {
        preset: Aura,
      },
    },
  },

  compatibilityDate: '2025-05-11',
});
