<script lang="ts">
  // Background fill-sweep card: a gradient layer fills left→right behind the
  // content to episodeOverall(ep), shifting from the series accent toward green
  // as it nears completion. A green ✓ appears when the episode is archived.
  //
  // Used on the series page (full episode card) and in the Activity drawer
  // list (compact active-episode card). `compact` tightens the padding/type.
  import type { EpisodeDetail } from '$lib/api'
  import { episodeOverall, episodeStage } from '$lib/pipeline.svelte'
  import { fillGreenMix } from '$lib/pipeline-math'
  import { sseState } from '$lib/sse.svelte'
  import { formatBytes } from '$lib/utils'

  let {
    ep,
    compact = false,
    onclick,
  }: {
    ep: EpisodeDetail
    compact?: boolean
    onclick?: () => void
  } = $props()

  const overall = $derived(episodeOverall(ep))
  const stage = $derived(episodeStage(ep))
  const done = $derived(stage === 'done')
  const errored = $derived(stage === 'error')
  // 0 = accent, 1 = green. Drives the fill tint via color-mix in the style.
  const greenMix = $derived(fillGreenMix(overall, done))

  // Live download tick (bytes / speed / peers) when this episode is downloading.
  const dl = $derived(sseState.downloadProgress[ep.id] ?? null)
  // Live encode tick (resolution + speed) for whichever output is encoding.
  const enc = $derived.by(() => {
    for (const o of ep.outputs) {
      const p = sseState.encodeProgress[o.id]
      if (p?.percent != null) return p
    }
    return null
  })

  const stageLabel = $derived(
    errored
      ? 'Error'
      : done
        ? 'Archived'
        : stage === 'downloading'
          ? 'Downloading'
          : stage === 'encoding'
            ? 'Encoding'
            : 'Queued',
  )

  const detailLine = $derived.by(() => {
    if (errored) return ep.error_message ?? 'Failed'
    if (stage === 'downloading' && dl) {
      const speed = dl.speed_bps ? `${formatBytes(dl.speed_bps)}/s` : ''
      const peers = dl.peers ? `${dl.peers} peers` : ''
      return [speed, peers].filter(Boolean).join(' · ')
    }
    if (stage === 'encoding' && enc) {
      return [enc.resolution ? `${enc.resolution}p` : '', enc.speed ?? '']
        .filter(Boolean)
        .join(' · ')
    }
    return ''
  })

  const title = $derived(
    ep.title ?? (ep.episode_no != null ? `Episode ${ep.episode_no}` : 'Special'),
  )
</script>

<button
  type="button"
  {onclick}
  class="group relative block w-full overflow-hidden border text-left transition-colors duration-200
    {errored
      ? 'border-[var(--color-error)]/40'
      : 'border-[var(--color-border)] hover:border-[var(--color-border-strong)]'}
    {compact ? 'p-3' : 'p-3.5'}"
  style="
    --fill-color: color-mix(in oklab, var(--accent-text) {(1 - greenMix) * 100}%, var(--color-success) {greenMix * 100}%);
    --fill-pct: {overall}%;
    background: var(--color-surface);
  "
  aria-label={`${title} — ${stageLabel} ${Math.round(overall)}%`}
>
  <!-- Fill-sweep layer: a soft accent→green wash filling to --fill-pct. -->
  <div
    class="pointer-events-none absolute inset-0 z-0 transition-[width] duration-700 ease-[cubic-bezier(0.32,0.72,0,1)]"
    style="
      width: var(--fill-pct);
      background: linear-gradient(
        90deg,
        rgb(from var(--fill-color) r g b / 0.22),
        rgb(from var(--fill-color) r g b / 0.10) 70%,
        rgb(from var(--fill-color) r g b / 0.16)
      );
    "
  ></div>
  <!-- Leading edge highlight at the fill front. -->
  {#if overall > 0 && overall < 100}
    <div
      class="pointer-events-none absolute inset-y-0 z-0 w-px transition-[left] duration-700 ease-[cubic-bezier(0.32,0.72,0,1)]"
      style="left: var(--fill-pct); background: rgb(from var(--fill-color) r g b / 0.55);"
    ></div>
  {/if}

  <!-- ✓ completion badge -->
  {#if done}
    <span
      class="absolute right-2.5 top-2.5 z-10 inline-flex h-5 w-5 items-center justify-center bg-[var(--color-success)] text-black shadow-[0_2px_8px_-2px_var(--color-success)]"
      aria-hidden="true"
    >
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><path d="M20 6 9 17l-5-5" stroke-linecap="round" stroke-linejoin="round"/></svg>
    </span>
  {/if}

  <!-- Content -->
  <div class="relative z-[5] flex items-center gap-3">
    <span
      class="shrink-0 font-bold tabular-nums text-[var(--color-text-dim)] {compact ? 'text-[11px]' : 'text-xs'}"
    >
      {ep.episode_no != null ? `E${String(ep.episode_no).padStart(2, '0')}` : 'SP'}
    </span>

    <div class="min-w-0 flex-1">
      <p class="truncate font-medium text-[var(--color-text)] {compact ? 'text-[13px]' : 'text-sm'}">
        {title}
      </p>
      <div class="mt-0.5 flex items-center gap-2 text-[11px]">
        <span
          class="font-medium {errored ? 'text-[var(--color-error)]' : done ? 'text-[var(--color-success)]' : 'text-[var(--color-text-dim)]'}"
        >{stageLabel}</span>
        {#if detailLine}
          <span class="truncate text-[var(--color-muted)] tabular-nums">{detailLine}</span>
        {/if}
      </div>
    </div>

    {#if !done}
      <span class="shrink-0 tabular-nums font-semibold text-[var(--color-text-dim)] {compact ? 'text-xs' : 'text-sm'}">
        {Math.round(overall)}%
      </span>
    {/if}
  </div>
</button>
