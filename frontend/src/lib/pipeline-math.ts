// Pure pipeline-progress math — no Svelte runes, no store reads, no I/O.
// Everything live is passed in explicitly so these functions are unit-testable
// in isolation (see pipeline-math.test.ts). The reactive wrappers that read the
// SSE store live in pipeline.svelte.ts.
//
// Progress model (one continuous 0..100 bar per episode):
//   queued                 → 0
//   downloading            → download.percent * 0.5            (first half)
//   downloaded             → 50
//   encoding               → 50 + encodeAvg * 0.5              (second half)
//   encoded | thumbnailing → ~100 (bar full, no ✓ yet)
//   archived               → 100 (+ ✓)
//   error                  → frozen at the last computed %
//
// encodeAvg is the mean of the episode's outputs' percents, where a finished
// output (encoded/archived) counts 100 and a not-yet-started (queued) output
// counts 0 — so the episode bar advances smoothly as outputs finish
// sequentially.

export type PipelineStatus =
  | 'queued'
  | 'downloading'
  | 'downloaded'
  | 'encoding'
  | 'encoded'
  | 'thumbnailing'
  | 'archived'
  | 'error'
  | (string & {})

export type Stage = 'queued' | 'downloading' | 'encoding' | 'done' | 'error'

/** Live per-output state used by the encode-average math. */
export interface OutputLive {
  /** The output's effective status (SSE override falls back to the DB status). */
  status: string
  /** Live encode percent for this output (0..100) when it is actively encoding. */
  percent?: number | null
}

const clampPct = (n: number): number => (n < 0 ? 0 : n > 100 ? 100 : n)

/** An output counts as fully done (100%) once it has finished encoding. */
const OUTPUT_DONE = new Set(['encoded', 'archived'])

/**
 * Mean encode progress across an episode's outputs (0..100):
 *   - a finished output (encoded/archived) counts 100
 *   - a queued/pending output counts 0
 *   - an encoding output counts its live percent (0 when unknown)
 * Returns 0 when the episode has no outputs.
 */
export function encodeAvg(outputs: OutputLive[]): number {
  if (outputs.length === 0) return 0
  let sum = 0
  for (const o of outputs) {
    if (OUTPUT_DONE.has(o.status)) {
      sum += 100
    } else if (o.status === 'encoding') {
      sum += clampPct(o.percent ?? 0)
    } else {
      // queued / pending / error / unknown → no contribution
      sum += 0
    }
  }
  return sum / outputs.length
}

/** Live signals needed to compute an episode's overall progress. */
export interface EpisodeProgressInput {
  /** Effective status: SSE override over the DB status. */
  status: string
  /** Live download percent (0..100) when downloading; ignored otherwise. */
  downloadPercent?: number | null
  /** Live per-output state for the encode half. */
  outputs: OutputLive[]
}

/**
 * One continuous 0..100 bar for an episode. `error` freezes at the last
 * computed % rather than resetting — the caller passes the pre-error status's
 * inputs (download/outputs) so the frozen value reflects where it stalled; with
 * no such inputs it resolves to whatever the half-progress math yields.
 */
export function episodeOverall(input: EpisodeProgressInput): number {
  const { status, downloadPercent, outputs } = input
  switch (status) {
    case 'queued':
      return 0
    case 'downloading':
      return clampPct((downloadPercent ?? 0) * 0.5)
    case 'downloaded':
      return 50
    case 'encoding':
      return clampPct(50 + encodeAvg(outputs) * 0.5)
    case 'encoded':
    case 'thumbnailing':
      return 100
    case 'archived':
      return 100
    case 'error':
      // Freeze at the furthest point reached: if any outputs exist we were in
      // the encode half, otherwise fall back to the download half.
      if (outputs.length > 0) return clampPct(50 + encodeAvg(outputs) * 0.5)
      return clampPct((downloadPercent ?? 0) * 0.5)
    default:
      return 0
  }
}

/** Coarse stage for label + color. */
export function episodeStage(status: string): Stage {
  switch (status) {
    case 'downloading':
      return 'downloading'
    case 'downloaded':
    case 'encoding':
    case 'encoded':
    case 'thumbnailing':
      return 'encoding'
    case 'archived':
      return 'done'
    case 'error':
      return 'error'
    case 'queued':
    default:
      return 'queued'
  }
}

/** Statuses whose episodes appear in the "active now" surfaces. */
export const ACTIVE_STATUSES = new Set(['downloading', 'encoding'])

export function isActive(status: string): boolean {
  return ACTIVE_STATUSES.has(status)
}

/**
 * Mean episodeOverall across the given active inputs, or null when the list is
 * empty (drives the taskbar; null clears it). Callers pass only the inputs they
 * consider active.
 */
export function overallPercentOf(inputs: EpisodeProgressInput[]): number | null {
  if (inputs.length === 0) return null
  let sum = 0
  for (const i of inputs) sum += episodeOverall(i)
  return sum / inputs.length
}
