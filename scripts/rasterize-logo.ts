/**
 * Rasterizes the ssanime logo SVG to PNG at required sizes.
 * Usage: bun scripts/rasterize-logo.ts
 * Requires: @resvg/resvg-js
 */

import { Resvg } from '@resvg/resvg-js'
import { readFileSync, writeFileSync, mkdirSync } from 'fs'
import { join, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const root = join(__dirname, '..')

// The standalone mark SVG (with bg) is the source for rasterization
const markSvg = readFileSync(join(root, 'frontend/public/favicon.svg'), 'utf8')

function renderPng(svg: string, size: number): Buffer {
  const resvg = new Resvg(svg, {
    fitTo: { mode: 'width', value: size },
    background: 'transparent',
  })
  return Buffer.from(resvg.render().asPng())
}

// 1024×1024 master
const master = renderPng(markSvg, 1024)
writeFileSync(join(root, 'frontend/src/lib/assets/logo-1024.png'), master)
console.log('wrote frontend/src/lib/assets/logo-1024.png (1024×1024)')

// 32×32 tray icon
const tray32 = renderPng(markSvg, 32)
const trayIconPath = join(root, 'internal/tray/icon/icon.png')
writeFileSync(trayIconPath, tray32)
console.log('wrote internal/tray/icon/icon.png (32×32)')

// Also write desktop/src-tauri source icon (will be input to tauri icon command)
const tauri1024 = renderPng(markSvg, 1024)
writeFileSync(join(root, 'desktop/src-tauri/icons/icon.png'), tauri1024)
console.log('wrote desktop/src-tauri/icons/icon.png (1024×1024)')

console.log('Done.')
