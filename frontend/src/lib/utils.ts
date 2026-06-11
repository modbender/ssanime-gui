import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

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

export function derivedStatusColor(status: string): string {
  switch (status) {
    case 'completed': return 'bg-green-500/15 text-green-300 border-green-500/30'
    case 'airing': return 'bg-blue-500/15 text-blue-300 border-blue-500/30'
    case 'active': return 'bg-blue-500/15 text-blue-300 border-blue-500/30'
    case 'up_to_date': return 'bg-cyan-500/15 text-cyan-300 border-cyan-500/30'
    case 'incomplete': return 'bg-yellow-500/15 text-yellow-300 border-yellow-500/30'
    case 'not_aired': return 'bg-gray-500/15 text-gray-400 border-gray-500/30'
    case 'cancelled': return 'bg-red-500/15 text-red-300 border-red-500/30'
    case 'paused': return 'bg-amber-500/15 text-amber-300 border-amber-500/30'
    case 'dropped': return 'bg-rose-500/15 text-rose-300 border-rose-500/30'
    case 'error': return 'bg-red-500/15 text-red-300 border-red-500/30'
    default: return 'bg-gray-500/15 text-gray-400 border-gray-500/30'
  }
}

export function derivedStatusLabel(status: string): string {
  const map: Record<string, string> = {
    completed: 'Completed',
    airing: 'Airing',
    active: 'Active',
    up_to_date: 'Up to date',
    incomplete: 'Incomplete',
    not_aired: 'Not aired',
    cancelled: 'Cancelled',
    paused: 'Paused',
    dropped: 'Dropped',
    error: 'Error',
  }
  return map[status] ?? status
}

/**
 * The user-facing status of a tracked series, honoring the manual override
 * layer (user_status) over the automatic derived_status. A series with a
 * manual override (paused/dropped) shows that; otherwise the derived status.
 */
export function trackedStatus(s: {
  user_status?: string | null
  derived_status: string
}): string {
  if (s.user_status === 'paused' || s.user_status === 'dropped') return s.user_status
  return s.derived_status
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
