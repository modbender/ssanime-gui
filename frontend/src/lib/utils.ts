import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import type { WatchStatus } from '$lib/api'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/** Default accent (violet) used when a series has no cover_color. */
export const DEFAULT_ACCENT = '#7c6af0'

/** Parse a #rrggbb / #rgb hex into an "r g b" channel string for CSS color-mix / rgb(). */
export function hexToRgbChannels(hex: string | null | undefined): string {
  if (!hex) return '124 106 240'
  let h = hex.trim().replace('#', '')
  if (h.length === 3) h = h.split('').map((c) => c + c).join('')
  if (h.length !== 6 || /[^0-9a-fA-F]/.test(h)) return '124 106 240'
  const n = parseInt(h, 16)
  return `${(n >> 16) & 255} ${(n >> 8) & 255} ${n & 255}`
}

/**
 * Resolve a usable accent hex from a series' cover_color, with a graceful
 * fallback to the default violet when the value is null/blank/invalid.
 */
export function resolveAccent(coverColor: string | null | undefined): string {
  if (!coverColor) return DEFAULT_ACCENT
  const h = coverColor.trim()
  if (/^#?[0-9a-fA-F]{3}$|^#?[0-9a-fA-F]{6}$/.test(h)) {
    return h.startsWith('#') ? h : `#${h}`
  }
  return DEFAULT_ACCENT
}

/**
 * Pick a readable foreground (#0a0a0a near-black or #ffffff white) for text
 * placed on a solid accent fill. Resolves the accent the same way the rest of
 * the UI does, computes WCAG relative luminance, and returns dark text only for
 * genuinely light accents (e.g. Dr. STONE's cream cover). Falls back to white
 * for the default violet accent.
 */
export function accentForeground(coverColor: string | null | undefined): string {
  const hex = resolveAccent(coverColor)
  let h = hex.replace('#', '')
  if (h.length === 3) h = h.split('').map((c) => c + c).join('')
  const n = parseInt(h, 16)
  const channels = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((c) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  })
  const L = 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
  return L > 0.6 ? '#0a0a0a' : '#ffffff'
}

/** sRGB → relative luminance (WCAG), shared by the accent-foreground heuristics. */
function relativeLuminance(r: number, g: number, b: number): number {
  const lin = [r, g, b].map((c) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2]
}

function rgbToHsl(r: number, g: number, b: number): [number, number, number] {
  const rn = r / 255, gn = g / 255, bn = b / 255
  const max = Math.max(rn, gn, bn), min = Math.min(rn, gn, bn)
  const d = max - min
  let h = 0
  if (d !== 0) {
    if (max === rn) h = ((gn - bn) / d) % 6
    else if (max === gn) h = (bn - rn) / d + 2
    else h = (rn - gn) / d + 4
    h *= 60
    if (h < 0) h += 360
  }
  const l = (max + min) / 2
  const s = d === 0 ? 0 : d / (1 - Math.abs(2 * l - 1))
  return [h, s, l]
}

function hslToRgb(h: number, s: number, l: number): [number, number, number] {
  const c = (1 - Math.abs(2 * l - 1)) * s
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1))
  const m = l - c / 2
  let r = 0, g = 0, b = 0
  if (h < 60) [r, g, b] = [c, x, 0]
  else if (h < 120) [r, g, b] = [x, c, 0]
  else if (h < 180) [r, g, b] = [0, c, x]
  else if (h < 240) [r, g, b] = [0, x, c]
  else if (h < 300) [r, g, b] = [x, 0, c]
  else [r, g, b] = [c, 0, x]
  return [Math.round((r + m) * 255), Math.round((g + m) * 255), Math.round((b + m) * 255)]
}

/** Minimum relative luminance an accent must clear to stay legible as TEXT on the
 *  near-black app surfaces. ~0.18 corresponds to roughly a 4.5:1 contrast against
 *  the #0b0b0c page bg; the default violet (#7c6af0, L≈0.22) already clears it. */
const ACCENT_TEXT_MIN_L = 0.18

/**
 * Legibility-clamped accent for FOREGROUND use (accent-colored text, thin lines,
 * small dots/pips, rings, count chips) drawn directly on the app's near-black
 * surfaces. Preserves the accent's hue but lifts very DARK covers (navy,
 * near-black, deep maroon) until they clear a minimum perceived luminance, so
 * they don't vanish against the dark UI. Light/normal accents pass through
 * unchanged — only genuinely dark ones get raised.
 */
export function accentText(coverColor: string | null | undefined): string {
  const hex = resolveAccent(coverColor)
  let h = hex.replace('#', '')
  if (h.length === 3) h = h.split('').map((c) => c + c).join('')
  const n = parseInt(h, 16)
  let r = (n >> 16) & 255, g = (n >> 8) & 255, b = n & 255

  if (relativeLuminance(r, g, b) >= ACCENT_TEXT_MIN_L) {
    return `#${h}`
  }

  const [hue, sat0, l0] = rgbToHsl(r, g, b)
  // Very-low-saturation darks (near-black) would lift toward muddy grey; floor
  // saturation so the lifted result still reads as a tinted accent.
  const sat = Math.max(sat0, 0.35)
  // Raise lightness in small steps until the luminance clears the threshold,
  // capping at a bright-but-not-white ceiling.
  let l = l0
  for (let i = 0; i < 40 && l < 0.82; i++) {
    ;[r, g, b] = hslToRgb(hue, sat, l)
    if (relativeLuminance(r, g, b) >= ACCENT_TEXT_MIN_L) break
    l += 0.02
  }
  // Ensure a sane floor even if the loop bailed early.
  if (l < 0.62) l = 0.62
  ;[r, g, b] = hslToRgb(hue, sat, l)
  return `#${[r, g, b].map((c) => c.toString(16).padStart(2, '0')).join('')}`
}

/** The "r g b" channel string for the legibility-clamped accent (mirrors
 *  hexToRgbChannels) so it can drive rgb(var(--accent-text-rgb) / α) uses. */
export function accentTextRgb(coverColor: string | null | undefined): string {
  return hexToRgbChannels(accentText(coverColor))
}

export function formatBytes(bytes: number | null | undefined): string {
  if (bytes == null || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
}

export function formatDate(ts: number | null | undefined): string {
  if (!ts) return '—'
  return new Date(ts * 1000).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

export function statusColor(status: string): string {
  switch (status) {
    case 'archived': return 'text-green-400'
    case 'encoded': return 'text-blue-400'
    case 'encoding': return 'text-blue-300 animate-pulse'
    case 'downloaded': return 'text-cyan-400'
    case 'downloading': return 'text-cyan-300 animate-pulse'
    case 'queued': return 'text-yellow-400'
    case 'error': return 'text-red-400'
    default: return 'text-gray-400'
  }
}

/**
 * The user-facing bucket of a series. A subscribed series buckets by `Completed`
 * (derived: finished airing + all episodes archived, wins over the watch status)
 * else its watch status (`watching | on_hold | dropped`). An UNsubscribed series
 * that still has episodes (manually downloaded, never tracked) buckets as
 * `downloaded`. Returns one of `completed | watching | on_hold | dropped |
 * downloaded`, which feed the label/color helpers.
 */
export function watchBucket(s: {
  status?: WatchStatus | null
  derived_status?: string
  subscribed?: boolean
  episode_total?: number
}): 'completed' | 'downloaded' | WatchStatus {
  if (!s.subscribed && (s.episode_total ?? 0) > 0) return 'downloaded'
  if (s.derived_status === 'completed') return 'completed'
  return s.status ?? 'watching'
}

/** Display color (chip classes) for a watch bucket. */
export function watchStatusColor(bucket: string): string {
  switch (bucket) {
    case 'completed': return 'bg-green-500/15 text-green-300 border-green-500/30'
    case 'watching': return 'bg-blue-500/15 text-blue-300 border-blue-500/30'
    case 'downloaded': return 'bg-cyan-500/15 text-cyan-300 border-cyan-500/30'
    case 'on_hold': return 'bg-amber-500/15 text-amber-300 border-amber-500/30'
    case 'dropped': return 'bg-rose-500/15 text-rose-300 border-rose-500/30'
    default: return 'bg-gray-500/15 text-gray-400 border-gray-500/30'
  }
}

/** Display label for a watch bucket. */
export function watchStatusLabel(bucket: string): string {
  const map: Record<string, string> = {
    completed: 'Completed',
    watching: 'Watching',
    downloaded: 'Downloaded',
    on_hold: 'On Hold',
    dropped: 'Dropped',
  }
  return map[bucket] ?? bucket
}

/**
 * Coerce an air-date value into epoch milliseconds. Accepts a unix-seconds
 * number, an ISO date string ("2012-04-09"), or null. Returns null if unusable.
 */
function toEpochMs(value: string | number | null | undefined): number | null {
  if (value == null || value === '') return null
  if (typeof value === 'number') return value * 1000
  const ms = Date.parse(value)
  return Number.isNaN(ms) ? null : ms
}

/** True when the given air date is still in the future. */
export function isFuture(value: string | number | null | undefined): boolean {
  const ms = toEpochMs(value)
  return ms != null && ms > Date.now()
}

/**
 * Human relative time for an air date — "3 days ago", "in 2 days", "today".
 * Accepts unix-seconds, an ISO date string, or null (→ '').
 */
export function relativeTime(value: string | number | null | undefined): string {
  const ms = toEpochMs(value)
  if (ms == null) return ''
  const diff = ms - Date.now()
  const future = diff > 0
  const abs = Math.abs(diff)
  const min = 60_000, hour = 3_600_000, day = 86_400_000
  if (abs < hour) {
    const m = Math.max(1, Math.round(abs / min))
    return future ? `in ${m} min` : `${m} min ago`
  }
  if (abs < day) {
    const h = Math.round(abs / hour)
    return future ? `in ${h}h` : `${h}h ago`
  }
  const d = Math.round(abs / day)
  if (d < 30) {
    const unit = d === 1 ? 'day' : 'days'
    return future ? `in ${d} ${unit}` : `${d} ${unit} ago`
  }
  const mo = Math.round(d / 30)
  if (mo < 12) {
    const unit = mo === 1 ? 'month' : 'months'
    return future ? `in ${mo} ${unit}` : `${mo} ${unit} ago`
  }
  const y = Math.round(d / 365)
  const unit = y === 1 ? 'year' : 'years'
  return future ? `in ${y} ${unit}` : `${y} ${unit} ago`
}

/**
 * Compact countdown to a future unix-seconds timestamp — "3d 14h", "14h 30m",
 * "30m". Returns '' once the moment has passed.
 */
export function countdown(unixSeconds: number | null | undefined): string {
  if (unixSeconds == null) return ''
  let secs = unixSeconds - Math.floor(Date.now() / 1000)
  if (secs <= 0) return ''
  const d = Math.floor(secs / 86_400); secs -= d * 86_400
  const h = Math.floor(secs / 3_600); secs -= h * 3_600
  const m = Math.floor(secs / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

/** Format an AniList media format/status token (UPPER_SNAKE) for display. */
export function titleCase(s: string | null | undefined): string {
  if (!s) return ''
  return s
    .toLowerCase()
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
}
