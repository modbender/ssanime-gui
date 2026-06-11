<script lang="ts">
  import { navigate } from 'svelte-routing'
  import type { SeriesProgress } from '$lib/api'
  import { derivedStatusColor, derivedStatusLabel, formatBytes, trackedStatus } from '$lib/utils'
  import { sseState } from '$lib/sse.svelte'

  let {
    series,
    width = '',
    showProgress = false,
  }: {
    series: SeriesProgress
    /** optional fixed width, e.g. 'w-[150px]' for carousels; omit to fill grid cell */
    width?: string
    /** when true, overlay live SSE download/encode progress for this series */
    showProgress?: boolean
  } = $props()

  const title = $derived(series.english_title || series.romaji_title || series.title)
  const status = $derived(trackedStatus(series))

  // Live progress for the series' currently-active episode. SSE state is keyed by
  // episode id; both payloads carry series_id, so we pick the most-advanced one.
  const live = $derived.by(() => {
    if (!showProgress) return null
    let best: { kind: 'download' | 'encode'; percent: number } | null = null
    for (const p of Object.values(sseState.downloadProgress)) {
      if (p.series_id === series.id && !p.done) {
        if (!best || p.percent > best.percent) best = { kind: 'download', percent: p.percent }
      }
    }
    for (const p of Object.values(sseState.encodeProgress)) {
      if (p.series_id === series.id && typeof p.percent === 'number') {
        // encoding supersedes download in the pipeline → prefer it
        best = { kind: 'encode', percent: p.percent }
      }
    }
    return best
  })
</script>

<button
  class="group block text-left w-full {width}"
  onclick={() => navigate(`/series/${series.id}`)}
>
  <!-- Poster: double-bezel — outer shell holds the lift + glow -->
  <div
    class="relative aspect-[2/3] overflow-hidden bg-[var(--color-surface-2)] ring-1 ring-white/[0.06]
           transition-[transform,box-shadow] duration-500 ease-[cubic-bezier(0.32,0.72,0,1)]
           group-hover:-translate-y-1.5 group-hover:shadow-[0_22px_45px_-18px_rgba(0,0,0,0.85)] group-hover:ring-white/15"
  >
    {#if series.cover_image_url}
      <img
        src={series.cover_image_url}
        alt={title}
        loading="lazy"
        class="w-full h-full object-cover transition-transform duration-700 ease-[cubic-bezier(0.32,0.72,0,1)] group-hover:scale-[1.07]"
      />
    {:else}
      <div class="w-full h-full flex items-center justify-center text-[var(--color-faint)]">
        <svg width="38" height="38" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.25"><rect x="3" y="3" width="18" height="18" rx="3"/><path d="m3 16 5-5 4 4 3-3 6 6" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </div>
    {/if}

    <!-- bottom scrim for legibility -->
    <div class="absolute inset-x-0 bottom-0 h-2/5 bg-gradient-to-t from-black/85 via-black/30 to-transparent opacity-90"></div>

    <!-- top-right flags -->
    <div class="absolute top-2 right-2 flex flex-col gap-1.5">
      {#if series.subscribed}
        <span class="w-6 h-6 bg-[var(--accent)] shadow-lg flex items-center justify-center" title="Subscribed">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2.25"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9M13.7 21a2 2 0 0 1-3.4 0" stroke-linecap="round" stroke-linejoin="round"/></svg>
        </span>
      {/if}
      {#if series.favorite}
        <span class="w-6 h-6 bg-amber-400 shadow-lg flex items-center justify-center" title="Favorite">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="#1a1407" stroke="none"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>
        </span>
      {/if}
    </div>

    <!-- bottom overlay: status pill + ep count -->
    <div class="absolute inset-x-0 bottom-0 p-2.5 flex items-end justify-between gap-2">
      <span class={`inline-flex items-center border px-2 py-0.5 text-[10px] font-medium leading-none tracking-tight backdrop-blur-sm ${derivedStatusColor(status)}`}>
        {derivedStatusLabel(status)}
      </span>
      <span class="text-[10px] font-semibold text-white/90 tabular-nums drop-shadow">
        {series.episode_archived}/{series.episode_total}
      </span>
    </div>

    <!-- live progress strip (only on the Currently-downloading row) -->
    {#if live}
      <div class="absolute inset-x-0 bottom-0 h-1 bg-black/40">
        <div
          class="h-full {live.kind === 'encode' ? 'bg-[var(--color-info)]' : 'bg-[var(--accent)]'} transition-[width] duration-500 ease-[cubic-bezier(0.32,0.72,0,1)]"
          style="width: {Math.max(2, Math.min(100, live.percent))}%"
        ></div>
      </div>
    {/if}
  </div>

  <!-- meta -->
  <div class="mt-2.5 px-0.5 space-y-0.5">
    <p class="text-[13px] font-medium leading-snug text-[var(--color-text)] line-clamp-1 transition-colors duration-200 group-hover:text-[var(--accent)]">
      {title}
    </p>
    {#if series.space_saved_bytes > 0}
      <p class="text-[11px] text-[var(--color-success)] font-medium">{formatBytes(series.space_saved_bytes)} saved</p>
    {:else}
      <p class="text-[11px] text-[var(--color-muted)]">{series.format ?? 'Series'}</p>
    {/if}
  </div>
</button>
