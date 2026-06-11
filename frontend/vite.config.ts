import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    tailwindcss(),
    svelte(),
  ],
  // Absolute base: the daemon serves the SPA at /; a relative base breaks
  // asset resolution on hard-loads of nested routes like /series/anilist/:id.
  base: '/',
  build: {
    outDir: path.resolve(__dirname, '../internal/server/dist'),
    emptyOutDir: true,
  },
  resolve: {
    alias: {
      '$lib': path.resolve(__dirname, './src/lib'),
    },
  },
})
