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
  base: './',
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
