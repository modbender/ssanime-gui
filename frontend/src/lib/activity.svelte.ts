// Headless active-episode state — feeds the OS taskbar / favicon progress ring.
//
// Holds the union of currently-active episodes (downloading + encoding) across
// all subscribed series, sourced from /api/activity and kept fresh against SSE
// status events. App.svelte reads `activeEpisodes` to drive the taskbar; the
// drawer UI that previously consumed this has been removed.

import { api, type EpisodeDetail } from '$lib/api'
import { sseState } from '$lib/sse.svelte'
import { isActive } from '$lib/pipeline-math'

export const activityState = $state({
  /** Active episodes (downloading + encoding) from the last activity snapshot. */
  activeEpisodes: [] as EpisodeDetail[],
})

let inFlight = false

/**
 * Refetch the activity snapshot and reduce it to the active episodes. Guarded
 * against re-entry by a plain (non-reactive) flag so a status-driven caller can
 * never self-trigger. Each episode carries `series_id` + `series_title`.
 */
export async function refreshActive(): Promise<void> {
  if (inFlight) return
  inFlight = true
  try {
    const res = await api.getActivity()
    const active: EpisodeDetail[] = []
    for (const s of res.series ?? []) {
      for (const ep of s.episodes ?? []) {
        if (isActive(sseState.episodeStatus[ep.id] ?? ep.status)) active.push(ep)
      }
    }
    activityState.activeEpisodes = active
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
