// Activity drawer — shared open/selected state + the live active-episode set.
//
// One reactive module owns: whether the drawer is open, which episode (if any)
// is selected for detail mode, and the union of currently-active episodes
// (downloading + encoding) fetched from /api/queue and kept fresh against SSE
// status events. Both the drawer and the taskbar read `activeEpisodes` so the
// queue snapshot is fetched once, not twice.

import { api, type EpisodeDetail } from '$lib/api'
import { sseState } from '$lib/sse.svelte'

export const activityState = $state({
  open: false,
  /** Episode id in detail mode, or null for list mode. */
  selectedEpisodeId: null as number | null,
  /** Active episodes (downloading + encoding) from the last queue snapshot. */
  activeEpisodes: [] as EpisodeDetail[],
})

/** Open the drawer. Pass an episode id to land directly in detail mode. */
export function openDrawer(episodeId?: number): void {
  activityState.selectedEpisodeId = episodeId ?? null
  activityState.open = true
}

export function closeDrawer(): void {
  activityState.open = false
}

export function toggleDrawer(): void {
  if (activityState.open) closeDrawer()
  else openDrawer()
}

/** Drop back from detail mode to the active-episode list. */
export function backToList(): void {
  activityState.selectedEpisodeId = null
}

export function selectEpisode(episodeId: number): void {
  activityState.selectedEpisodeId = episodeId
}

let inFlight = false

/**
 * Refetch the queue snapshot into `activeEpisodes`. Guarded against re-entry by
 * a plain (non-reactive) flag so a status-driven caller can never self-trigger.
 * Both lists carry `series_title`, so the drawer can group by series.
 */
export async function refreshActive(): Promise<void> {
  if (inFlight) return
  inFlight = true
  try {
    const q = await api.getQueue()
    activityState.activeEpisodes = [...q.downloading, ...q.encoding]
  } catch {
    // Transient daemon hiccup — keep the last snapshot; SSE keeps %/status live.
  } finally {
    inFlight = false
  }
}

/** Background polling lifecycle: prime once, then refetch on SSE status churn. */
let started = false

export function startActivity(): () => void {
  if (started) return () => {}
  started = true
  refreshActive()

  // A status transition (queued→downloading→…→archived) changes which episodes
  // are active; the count of distinct status keys is a cheap change signal.
  let lastSignal = -1
  const stop = $effect.root(() => {
    $effect(() => {
      const signal = Object.values(sseState.episodeStatus).join('|').length
      if (signal === lastSignal) return
      lastSignal = signal
      refreshActive()
    })
  })

  return () => {
    stop()
    started = false
  }
}
