// SSE client — connects to /api/events and feeds reactive state.
// Uses Svelte 5 $state rune so components auto-update.

import { toast } from './toast.svelte'

export interface DownloadProgress {
  episode_id: number
  series_id: number
  bytes_done: number
  bytes_total: number
  speed_bps: number
  percent: number
  peers: number
  done: boolean
}

// Two emitters populate this: the per-tick progress emitter sends percent+speed,
// the output-status emitter sends status. percent/speed/status are therefore optional.
export interface EncodeProgress {
  episode_id: number
  series_id: number
  output_id: number
  resolution: number
  percent: number
  speed?: string
  status?: string
}

export interface EpisodeStatus {
  episode_id: number
  status: string
  error_message?: string
}

export interface LogEvent {
  level: string
  message: string
  ts: number
  /** monotonic client-side id; ts is seconds-granular so same-second events would otherwise collide as list keys */
  seq: number
}

// Global reactive SSE state.
//   downloadProgress — keyed by episode_id (one in-flight download per episode)
//   encodeProgress   — keyed by output_id (outputs encode sequentially, one row
//                       per target resolution; keying by episode_id would make
//                       multi-resolution progress last-writer-wins)
//   episodeStatus    — keyed by episode_id
//   outputStatus     — keyed by output_id (per-output status transitions arrive
//                       on encode.progress with a `status` field and no percent)
export const sseState = $state({
  connected: false,
  downloadProgress: {} as Record<number, DownloadProgress>,
  encodeProgress: {} as Record<number, EncodeProgress>,
  episodeStatus: {} as Record<number, string>,
  outputStatus: {} as Record<number, string>,
  logs: [] as LogEvent[],
  lastHeartbeat: 0,
})

let es: EventSource | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
let logSeq = 0

function connect() {
  if (es) {
    es.close()
  }
  es = new EventSource('/api/events')

  es.onopen = () => {
    sseState.connected = true
  }

  es.onerror = () => {
    sseState.connected = false
    es?.close()
    es = null
    // Reconnect after 3s
    reconnectTimer = setTimeout(connect, 3000)
  }

  es.addEventListener('download.progress', (e) => {
    try {
      const data: DownloadProgress = JSON.parse(e.data)
      sseState.downloadProgress[data.episode_id] = data
    } catch {}
  })

  es.addEventListener('encode.progress', (e) => {
    try {
      const data: EncodeProgress = JSON.parse(e.data)
      sseState.encodeProgress[data.output_id] = data
      if (data.status != null) {
        sseState.outputStatus[data.output_id] = data.status
      }
    } catch {}
  })

  es.addEventListener('episode.status', (e) => {
    try {
      const data: EpisodeStatus = JSON.parse(e.data)
      sseState.episodeStatus[data.episode_id] = data.status
    } catch {}
  })

  es.addEventListener('log', (e) => {
    try {
      const data: LogEvent = { ...JSON.parse(e.data), seq: logSeq++ }
      sseState.logs = [data, ...sseState.logs].slice(0, 500)
    } catch {}
  })

  es.addEventListener('extensions.updated', (e) => {
    try {
      const data: { count: number } = JSON.parse(e.data)
      if (data.count > 0) {
        toast.info(`Updated ${data.count} extension${data.count === 1 ? '' : 's'}`)
      }
    } catch {}
  })

  es.addEventListener('heartbeat', () => {
    sseState.lastHeartbeat = Date.now()
    sseState.connected = true
  })

  // Generic message fallback
  es.onmessage = (e) => {
    try {
      const data = JSON.parse(e.data)
      if (data.type === 'heartbeat') {
        sseState.lastHeartbeat = Date.now()
      }
    } catch {}
  }
}

export function startSSE() {
  connect()
  return () => {
    if (reconnectTimer) clearTimeout(reconnectTimer)
    es?.close()
    es = null
  }
}
