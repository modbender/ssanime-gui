// Derived pipeline state — bridges the raw SSE store (sse.svelte.ts) and the
// REST-fetched EpisodeDetail rows into the pure progress math (pipeline-math.ts).
//
// These selectors read the reactive `sseState`, so calling them inside a Svelte
// `$derived`/template re-runs them when SSE events arrive. The percent math
// itself lives in pipeline-math.ts and is unit-tested in isolation; this module
// only assembles the live inputs (status overrides, download %, per-output %).

import type { EpisodeDetail } from '$lib/api'
import { sseState } from '$lib/sse.svelte'
import {
  encodeAvg,
  episodeOverall as episodeOverallPure,
  episodeStage as episodeStagePure,
  isActive,
  overallPercentOf,
  type EpisodeProgressInput,
  type OutputLive,
  type Stage,
} from '$lib/pipeline-math'

export type { Stage } from '$lib/pipeline-math'

/** Live status for an episode: SSE override wins over the DB-fetched status. */
export function liveStatus(ep: EpisodeDetail): string {
  return sseState.episodeStatus[ep.id] ?? ep.status
}

/** Live status for a single output: SSE override wins over the DB status. */
function liveOutputStatus(ep: EpisodeDetail, outputId: number, dbStatus: string): string {
  return sseState.outputStatus[outputId] ?? dbStatus
}

/** Assemble the live per-output inputs for an episode from the SSE store. */
function outputLive(ep: EpisodeDetail): OutputLive[] {
  return ep.outputs.map((o) => ({
    status: liveOutputStatus(ep, o.id, o.status),
    percent: sseState.encodeProgress[o.id]?.percent ?? null,
  }))
}

/** Live progress inputs for an episode (status override + download% + outputs). */
function progressInput(ep: EpisodeDetail): EpisodeProgressInput {
  return {
    status: liveStatus(ep),
    downloadPercent: sseState.downloadProgress[ep.id]?.percent ?? null,
    outputs: outputLive(ep),
  }
}

/** One continuous 0..100 bar for an episode, reading live SSE state. */
export function episodeOverall(ep: EpisodeDetail): number {
  return episodeOverallPure(progressInput(ep))
}

/** Coarse stage ('queued'|'downloading'|'encoding'|'done'|'error') for label/color. */
export function episodeStage(ep: EpisodeDetail): Stage {
  return episodeStagePure(liveStatus(ep))
}

/** Mean live encode progress across an episode's outputs (0..100). */
export function episodeEncodeAvg(ep: EpisodeDetail): number {
  return encodeAvg(outputLive(ep))
}

export interface ActiveSeriesGroup {
  seriesId: number
  seriesTitle: string
  episodes: EpisodeDetail[]
}

/**
 * Episodes whose live status is downloading or encoding, grouped by series.
 * The caller passes the full set of known episodes it has (e.g. the union of the
 * queue snapshot's downloading + encoding lists, or all episodes on a page);
 * grouping uses each episode's `series_id` + `series_title`. Series insertion
 * order is preserved.
 */
export function activeBySeries(eps: EpisodeDetail[]): ActiveSeriesGroup[] {
  const groups = new Map<number, ActiveSeriesGroup>()
  for (const ep of eps) {
    if (!isActive(liveStatus(ep))) continue
    let g = groups.get(ep.series_id)
    if (!g) {
      g = { seriesId: ep.series_id, seriesTitle: ep.series_title, episodes: [] }
      groups.set(ep.series_id, g)
    }
    g.episodes.push(ep)
  }
  return [...groups.values()]
}

/**
 * Mean episodeOverall across all currently-active episodes, or null when none
 * are active (drives the taskbar; null clears it). The caller passes the full
 * known-episode set; this filters to active and averages.
 */
export function overallPercent(eps: EpisodeDetail[]): number | null {
  const inputs = eps.filter((ep) => isActive(liveStatus(ep))).map(progressInput)
  return overallPercentOf(inputs)
}
