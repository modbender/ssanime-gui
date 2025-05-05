// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  devtools: { enabled: true },
  css: ['~/assets/css/main.css'],
  modules: ['nuxt-electron', '@nuxtjs/tailwindcss', '@nuxt/icon'],
  icon: {
    // Configure icon module
    size: '24px',
    class: 'icon',
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
  // Disable SSR for Electron app (recommended by nuxt-electron)
  ssr: false,

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

  // Add aliases for better module resolution
  alias: {
    '~': '.',
    '@': '.',
  },

  // Configure build options
  build: {
    transpile: [],
  },

  // Ensure Vue compatibility
  vue: {
    compilerOptions: {
      isCustomElement: (tag) => ['webview'].includes(tag),
    },
  },

  // Explicitly activate pages module
  pages: true,

  // Improve HMR and error handling
  vite: {
    server: {
      hmr: {
        protocol: 'ws',
        host: 'localhost',
        port: 24678,
      },
    },
    clearScreen: false,
  },
});
