import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
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
    case 'up_to_date': return 'bg-cyan-500/15 text-cyan-300 border-cyan-500/30'
    case 'incomplete': return 'bg-yellow-500/15 text-yellow-300 border-yellow-500/30'
    case 'not_aired': return 'bg-gray-500/15 text-gray-400 border-gray-500/30'
    case 'cancelled': return 'bg-red-500/15 text-red-300 border-red-500/30'
    default: return 'bg-gray-500/15 text-gray-400 border-gray-500/30'
  }
}

export function derivedStatusLabel(status: string): string {
  const map: Record<string, string> = {
    completed: 'Completed',
    airing: 'Airing',
    up_to_date: 'Up to date',
    incomplete: 'Incomplete',
    not_aired: 'Not aired',
    cancelled: 'Cancelled',
  }
  return map[status] ?? status
}
