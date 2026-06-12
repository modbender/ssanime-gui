// OS-level overall progress: Windows taskbar (Tauri) + browser favicon-ring/title.
//
// Driven by a single 0..100 | null value (the mean of active jobs). null clears
// every sink. The Tauri path is a dynamic import so the browser bundle never
// pulls @tauri-apps/api; all failures are swallowed so progress UI can never
// block or crash the app.

/** True inside the Tauri webview (desktop shell), false in a plain browser. */
export function isTauri(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

// ---- Tauri taskbar sink ----

async function setTauriProgress(percent: number | null): Promise<void> {
  try {
    const { getCurrentWindow, ProgressBarStatus } = await import('@tauri-apps/api/window')
    const win = getCurrentWindow()
    if (percent === null) {
      await win.setProgressBar({ status: ProgressBarStatus.None })
    } else {
      await win.setProgressBar({
        status: ProgressBarStatus.Normal,
        progress: Math.max(0, Math.min(100, Math.round(percent))),
      })
    }
  } catch {
    // Non-Windows desktop, missing capability, or API drift — best-effort only.
  }
}

// ---- Browser favicon-ring + title sink ----

const FAVICON_ID = 'app-favicon'
let originalTitle: string | null = null
let originalFavicon: string | null = null

/** Read the accent (foreground-legible) hex from the live CSS variables. */
function accentHex(): string {
  if (typeof document === 'undefined') return '#7c6af0'
  const v = getComputedStyle(document.documentElement)
    .getPropertyValue('--accent-text')
    .trim()
  return v || '#7c6af0'
}

/**
 * Draw a 32x32 progress ring to a data URL. Pure given (percent, color) — used
 * by the favicon sink and unit-testable via a canvas stub. Returns null when no
 * 2D context is available (SSR / headless without canvas).
 */
export function drawRingDataURL(
  percent: number,
  color: string,
  canvas?: HTMLCanvasElement,
): string | null {
  if (typeof document === 'undefined') return null
  const c = canvas ?? document.createElement('canvas')
  c.width = 32
  c.height = 32
  const ctx = c.getContext('2d')
  if (!ctx) return null

  const cx = 16
  const cy = 16
  const r = 13
  const start = -Math.PI / 2
  const frac = Math.max(0, Math.min(100, percent)) / 100

  ctx.clearRect(0, 0, 32, 32)
  // Track
  ctx.beginPath()
  ctx.arc(cx, cy, r, 0, Math.PI * 2)
  ctx.strokeStyle = 'rgba(120,120,140,0.30)'
  ctx.lineWidth = 4
  ctx.stroke()
  // Progress arc
  ctx.beginPath()
  ctx.arc(cx, cy, r, start, start + frac * Math.PI * 2)
  ctx.strokeStyle = color
  ctx.lineWidth = 4
  ctx.lineCap = 'round'
  ctx.stroke()

  return c.toDataURL('image/png')
}

function ensureFaviconLink(): HTMLLinkElement | null {
  if (typeof document === 'undefined') return null
  let link = document.getElementById(FAVICON_ID) as HTMLLinkElement | null
  if (!link) {
    // Reuse the existing static favicon link if present; otherwise create one.
    const existing = document.querySelector(
      'link[rel="icon"]',
    ) as HTMLLinkElement | null
    link = existing ?? document.createElement('link')
    link.id = FAVICON_ID
    link.rel = 'icon'
    if (!existing) document.head.appendChild(link)
  }
  return link
}

function setBrowserProgress(percent: number | null): void {
  if (typeof document === 'undefined') return
  const link = ensureFaviconLink()
  if (originalTitle === null) originalTitle = document.title
  if (originalFavicon === null && link) originalFavicon = link.href

  if (percent === null) {
    if (originalTitle !== null) document.title = originalTitle
    if (link && originalFavicon !== null) link.href = originalFavicon
    return
  }

  const rounded = Math.round(percent)
  const base = originalTitle ?? document.title
  document.title = `(${rounded}%) ${base.replace(/^\(\d+%\)\s*/, '')}`

  const url = drawRingDataURL(rounded, accentHex(), undefined)
  if (link && url) link.href = url
}

// ---- Unified sink ----

/** Route an overall-percent value to whichever sink is active. */
export function setOverallProgress(percent: number | null): void {
  if (isTauri()) {
    void setTauriProgress(percent)
  } else {
    setBrowserProgress(percent)
  }
}

/** Restore the static favicon/title and clear the taskbar. Call on unmount. */
export function clearOverallProgress(): void {
  setOverallProgress(null)
}
