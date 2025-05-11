import tailwindcss from '@tailwindcss/vite';

// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  devtools: { enabled: true },
  css: ['~/assets/css/main.css', '~/assets/scss/tailwind.scss'],

  modules: [
    'nuxt-electron',
    '@nuxt/icon',
    'shadcn-nuxt',
    '@pinia/nuxt',
    '@nuxtjs/color-mode',
  ],

  vite: {
    plugins: [tailwindcss()],
  },

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

  shadcn: {
    /**
     * Prefix for all the imported component
     */
    prefix: '',
    /**
     * Directory that the component lives in.
     * @default "./components/ui"
     */
    componentDir: './components/ui',
  },

  compatibilityDate: '2025-05-11',
});
