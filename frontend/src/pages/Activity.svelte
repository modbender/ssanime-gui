<script lang="ts">
  import { navigate } from 'svelte-routing'
  import {
    api,
    type ActivitySeries,
    type EpisodeDetail,
    type WatchStatus,
  } from '$lib/api'
  import Button from '$lib/components/Button.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import { sseState } from '$lib/sse.svelte'
  import { toast } from '$lib/toast.svelte'
  import { episodeOverall, episodeStage, liveStatus } from '$lib/pipeline.svelte'
  import { isActive } from '$lib/pipeline-math'
  import {
    formatBytes,
    formatDate,
    statusColor,
    watchStatusColor,
    watchStatusLabel,
  } from '$lib/utils'
  import { scrollScrim } from '$lib/scrollScrim'

  let series = $state<ActivitySeries[]>([])
  let loading = $state(true)
  let error = $state('')

  // Plain (non-reactive) re-entry guard so a status-driven refetch can never
  // self-trigger; `seeded` keeps live refetches from flashing the spinner.
  let inFlight = false
  let seeded = false

  async function load() {
    if (inFlight) return
    inFlight = true
    if (!seeded) loading = true
    error = ''
    try {
      const res = await api.getActivity()
      series = res.series ?? []
    } catch (e: any) {
      error = e.message
    } finally {
      loading = false
      seeded = true
      inFlight = false
    }
  }

  // Prime once on mount.
  $effect(() => { load() })

  // Refetch when an episode-status event arrives (an episode transitioned
  // queued→…→archived/error, so the fetched rows need refreshing). Depends ONLY
  // on the joined status signal, never on `loading`, so it can't self-trigger.
  $effect(() => {
    const _signal = Object.values(sseState.episodeStatus).join('|')
    void _signal
    if (seeded) load()
  })

  // ---- Per-series UI state ----
  let collapsed = $state<Set<number>>(new Set())
  let statusBusy = $state<Set<number>>(new Set())
  let epBusy = $state<Set<number>>(new Set()) // episodes with retry/reveal in flight

  function toggleCollapsed(id: number) {
    const next = new Set(collapsed)
    if (next.has(id)) next.delete(id); else next.add(id)
    collapsed = next
  }

  // ---- Live-merged view of the fetched rows ----
  // The fetched series carry their full episode record; SSE state overlays the
  // live status/percent per episode via the pipeline selectors. We keep the
  // backend's ordering but re-float any series with a live-active episode to the
  // top so a freshly-started download rises even before the next refetch.
  function seriesHasActive(s: ActivitySeries): boolean {
    return s.episodes.some((ep) => isActive(liveStatus(ep)))
  }

  function seriesErrors(s: ActivitySeries): number {
    return s.episodes.filter((ep) => liveStatus(ep) === 'error').length
  }

  function activeCount(s: ActivitySeries): number {
    return s.episodes.filter((ep) => isActive(liveStatus(ep))).length
  }

  const orderedSeries = $derived.by(() => {
    // Stable partition: active-first, otherwise preserve backend order.
    const withActive: ActivitySeries[] = []
    const rest: ActivitySeries[] = []
    for (const s of series) {
      if (seriesHasActive(s)) withActive.push(s)
      else rest.push(s)
    }
    return [...withActive, ...rest]
  })

  function seriesSummary(s: ActivitySeries): { label: string; tone: string } {
    const active = activeCount(s)
    const errors = seriesErrors(s)
    if (active > 0) return { label: `${active} active`, tone: 'text-[var(--color-info)]' }
    if (errors > 0) return { label: `${errors} error${errors === 1 ? '' : 's'}`, tone: 'text-[var(--color-error)]' }
    return { label: 'up to date', tone: 'text-[var(--color-muted)]' }
  }

  const seriesTitle = (s: ActivitySeries) => s.english_title || s.romaji_title || s.title

  // ---- Status switch ----
  const WATCH_STATUSES: { value: WatchStatus; label: string }[] = [
    { value: 'watching', label: 'Watching' },
    { value: 'on_hold', label: 'On Hold' },
    { value: 'dropped', label: 'Dropped' },
  ]

  async function setStatus(s: ActivitySeries, status: WatchStatus) {
    if (s.status === status || statusBusy.has(s.id)) return
    const next = new Set(statusBusy); next.add(s.id); statusBusy = next
    // Optimistic: reflect immediately, reconcile on the response.
    series = series.map((x) => (x.id === s.id ? { ...x, status } : x))
    try {
      const res = await api.setSeriesStatus(s.id, status)
      series = series.map((x) => (x.id === s.id ? { ...x, ...res.series } : x))
    } catch (e: any) {
      toast.error(e.message)
      await load()
    } finally {
      const done = new Set(statusBusy); done.delete(s.id); statusBusy = done
    }
  }

  // ---- Episode actions ----
  async function retry(ep: EpisodeDetail) {
    if (epBusy.has(ep.id)) return
    const next = new Set(epBusy); next.add(ep.id); epBusy = next
    try {
      await api.retryEpisode(ep.id)
      await load()
    } catch (e: any) {
      toast.error(e.message)
    } finally {
      const done = new Set(epBusy); done.delete(ep.id); epBusy = done
    }
  }

  async function reveal(ep: EpisodeDetail) {
    if (epBusy.has(ep.id)) return
    const next = new Set(epBusy); next.add(ep.id); epBusy = next
    try {
      // Prefer an encoded output (the archived artifact); fall back to the source.
      const out = ep.outputs.find((o) => o.encoded_path)
      if (out) await api.revealOutput(out.id)
      else await api.revealEpisodeSource(ep.id)
    } catch (e: any) {
      toast.error(revealError(e))
    } finally {
      const done = new Set(epBusy); done.delete(ep.id); epBusy = done
    }
  }

  function revealError(e: any): string {
    const msg = String(e?.message ?? '')
    if (msg.includes('409')) return 'File was cleaned up or moved.'
    if (msg.includes('404')) return 'File path is not set yet.'
    if (msg.includes('403')) return 'File is outside the managed folders.'
    return msg || 'Could not open the file.'
  }

  // ---- Per-episode live tick (download bytes/speed or encode %) ----
  function liveTick(ep: EpisodeDetail) {
    const dl = sseState.downloadProgress[ep.id]
    if (dl) return { kind: 'download' as const, ...dl }
    for (const out of ep.outputs) {
      const enc = sseState.encodeProgress[out.id]
      if (enc) return { kind: 'encode' as const, ...enc }
    }
    return null
  }

  const totalActive = $derived(series.reduce((n, s) => n + activeCount(s), 0))
</script>

<div class="flex flex-col h-full overflow-y-auto" use:scrollScrim>
  <!-- Header -->
  <div class="sticky top-0 z-10 flex items-center justify-between px-6 sm:px-10 py-4 bg-transparent backdrop-blur-0 border-b border-transparent transition-[background-color,border-color,backdrop-filter] duration-300 [.scrolled_&]:bg-[var(--color-bg)]/85 [.scrolled_&]:backdrop-blur-md [.scrolled_&]:border-[var(--color-border)]">
    <div class="flex items-baseline gap-2.5">
      <h1 class="text-[15px] font-semibold tracking-tight">Activity</h1>
      {#if !loading && totalActive > 0}
        <span class="text-xs font-medium tabular-nums px-2 py-0.5 bg-[rgb(var(--accent-rgb)/0.14)] text-[var(--color-accent)]">{totalActive} active</span>
      {/if}
    </div>
    <Button variant="outline" size="sm" onclick={load} disabled={loading} title="Refresh activity">
      {#if loading}
        <Spinner size={12} />
      {:else}
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
          <path d="M1 4v6h6M23 20v-6h-6" stroke-linecap="round" stroke-linejoin="round"/>
          <path d="M20.49 9A9 9 0 0 0 5.64 5.64L1 10m22 4l-4.64 4.36A9 9 0 0 1 3.51 15" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      {/if}
      Refresh
    </Button>
  </div>

  {#if loading}
    <div class="flex flex-1 items-center justify-center text-[var(--color-muted)]">
      <Spinner size={28} />
    </div>
  {:else if error}
    <div class="flex flex-1 items-center justify-center text-[var(--color-error)] text-sm">{error}</div>
  {:else if series.length === 0}
    <!-- Empty state -->
    <div class="flex flex-1 flex-col items-center justify-center gap-5 px-6 text-center">
      <div class="w-16 h-16 bg-white/[0.04] ring-1 ring-white/10 flex items-center justify-center text-[var(--color-faint)]">
        <svg width="30" height="30" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.25"><path d="M13 2 3 14h7l-1 8 10-12h-7l1-8z" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </div>
      <div class="space-y-1.5">
        <h2 class="text-lg font-semibold tracking-tight">Nothing yet</h2>
        <p class="text-sm text-[var(--color-muted)] max-w-sm">Subscribe to a series from Home and its downloads and encodes will show up here.</p>
      </div>
      <Button size="lg" onclick={() => navigate('/')}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 10.5 12 3l9 7.5M5 9v10a1 1 0 0 0 1 1h4v-6h4v6h4a1 1 0 0 0 1-1V9" stroke-linecap="round" stroke-linejoin="round"/></svg>
        Browse discovery
      </Button>
    </div>
  {:else}
    <div class="px-6 sm:px-10 py-6 space-y-4 animate-fade-up">
      {#each orderedSeries as s (s.id)}
        {@const isOpen = !collapsed.has(s.id)}
        {@const summary = seriesSummary(s)}
        {@const bucketLabel = s.derived_status === 'completed' ? 'completed' : s.status}
        <section class="border border-[var(--color-border)] bg-[var(--color-surface)]">
          <!-- Group header -->
          <div class="flex items-center gap-3 px-4 py-3">
            <!-- collapse toggle + poster + title -->
            <button
              type="button"
              class="flex min-w-0 flex-1 items-center gap-3 text-left"
              onclick={() => toggleCollapsed(s.id)}
              aria-expanded={isOpen}
            >
              <svg
                class="shrink-0 text-[var(--color-muted)] transition-transform duration-200 {isOpen ? 'rotate-90' : ''}"
                width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true"
              ><path d="m9 18 6-6-6-6" stroke-linecap="round" stroke-linejoin="round"/></svg>

              <div class="h-12 w-9 shrink-0 overflow-hidden bg-[var(--color-surface-2)] ring-1 ring-white/[0.06]">
                {#if s.cover_image_url}
                  <img src={s.cover_image_url} alt="" loading="lazy" class="h-full w-full object-cover" />
                {/if}
              </div>

              <div class="min-w-0">
                <p class="truncate text-sm font-medium text-[var(--color-text)]">{seriesTitle(s)}</p>
                <div class="mt-0.5 flex items-center gap-2">
                  <span class="text-[11px] font-medium {summary.tone}">{summary.label}</span>
                  <span class="text-[11px] text-[var(--color-faint)] tabular-nums">{s.episode_archived}/{s.episode_total}</span>
                </div>
              </div>
            </button>

            <span class={`hidden shrink-0 items-center border px-2 py-0.5 text-[10px] font-medium leading-none tracking-tight sm:inline-flex ${watchStatusColor(bucketLabel)}`}>
              {watchStatusLabel(bucketLabel)}
            </span>

            <!-- Status switch -->
            <div
              class="flex shrink-0 overflow-hidden border border-[var(--color-border)] bg-[var(--color-surface-2)]"
              role="group"
              aria-label="Watch status"
            >
              {#each WATCH_STATUSES as opt (opt.value)}
                {@const on = s.status === opt.value}
                <button
                  type="button"
                  class="px-2.5 py-1 text-[11px] font-medium transition-colors
                    {on
                      ? 'bg-[rgb(var(--accent-rgb)/0.18)] text-[var(--color-text)]'
                      : 'text-[var(--color-muted)] hover:bg-white/5 hover:text-[var(--color-text)]'}
                    disabled:opacity-50 disabled:cursor-not-allowed"
                  aria-pressed={on}
                  disabled={statusBusy.has(s.id)}
                  onclick={() => setStatus(s, opt.value)}
                >{opt.label}</button>
              {/each}
            </div>
          </div>

          <!-- Episode rows -->
          {#if isOpen}
            <ul class="divide-y divide-[var(--color-border)]/60 border-t border-[var(--color-border)]">
              {#if s.episodes.length === 0}
                <li class="px-4 py-4 text-xs text-[var(--color-muted)]">No episodes yet.</li>
              {/if}
              {#each s.episodes as ep (ep.id)}
                {@const st = liveStatus(ep)}
                {@const stage = episodeStage(ep)}
                {@const overall = episodeOverall(ep)}
                {@const tick = liveTick(ep)}
                {@const encoded = ep.outputs.filter((o) => o.encoded_size != null)}
                <li class="px-4 py-3">
                  <div class="flex items-start gap-3">
                    <!-- state glyph -->
                    <span class="mt-0.5 shrink-0">
                      {#if stage === 'done'}
                        <span class="inline-flex h-4 w-4 items-center justify-center bg-[var(--color-success)] text-black" aria-hidden="true">
                          <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.5"><path d="M20 6 9 17l-5-5" stroke-linecap="round" stroke-linejoin="round"/></svg>
                        </span>
                      {:else if stage === 'error'}
                        <span class="inline-flex h-4 w-4 items-center justify-center text-[var(--color-error)]" aria-hidden="true">
                          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><path d="M12 9v4M12 17h.01" stroke-linecap="round"/></svg>
                        </span>
                      {:else}
                        <span class="inline-flex h-4 w-4 items-center justify-center" aria-hidden="true">
                          <span class="h-2 w-2 rounded-full {isActive(st) ? 'bg-[var(--color-info)] animate-pulse' : 'bg-[var(--color-faint)]'}"></span>
                        </span>
                      {/if}
                    </span>

                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-2">
                        {#if ep.episode_no != null}
                          <span class="shrink-0 text-[11px] font-bold tabular-nums text-[var(--color-text-dim)]">E{String(ep.episode_no).padStart(2, '0')}</span>
                        {/if}
                        <span class="truncate text-sm font-medium text-[var(--color-text)]">
                          {ep.title ?? (ep.episode_no != null ? `Episode ${ep.episode_no}` : 'Special')}
                        </span>
                        <span class="ml-auto shrink-0 text-[11px] font-medium {statusColor(st)}">{st}</span>
                      </div>

                      <!-- Active: live progress bar + stage + speed/% -->
                      {#if isActive(st)}
                        <div class="mt-2 space-y-1.5">
                          <div class="h-1.5 w-full overflow-hidden bg-white/[0.06]">
                            <div class="h-full bg-[var(--color-accent)] transition-[width] duration-500 ease-[cubic-bezier(0.32,0.72,0,1)]" style="width: {Math.max(2, overall)}%"></div>
                          </div>
                          <div class="flex justify-between text-[11px] tabular-nums text-[var(--color-muted)]">
                            <span>{stage === 'downloading' ? 'Downloading' : 'Encoding'} · {Math.round(overall)}%</span>
                            {#if tick && tick.kind === 'download'}
                              <span>{formatBytes(tick.speed_bps)}/s{tick.peers ? ` · ${tick.peers} peers` : ''}</span>
                            {:else if tick && tick.kind === 'encode'}
                              <span>{tick.resolution}p{tick.speed ? ` · ${tick.speed}` : ''}</span>
                            {/if}
                          </div>
                        </div>

                      <!-- Error: message + retry -->
                      {:else if stage === 'error'}
                        {#if ep.error_message}
                          <p class="mt-1 truncate text-xs text-[var(--color-error)]" title={ep.error_message}>{ep.error_message}</p>
                        {/if}
                        <div class="mt-2">
                          <Button size="sm" variant="outline" onclick={() => retry(ep)} disabled={epBusy.has(ep.id)}>
                            {#if epBusy.has(ep.id)}<Spinner size={12} />{:else}
                              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M1 4v6h6M23 20v-6h-6" stroke-linecap="round" stroke-linejoin="round"/><path d="M20.49 9A9 9 0 0 0 5.64 5.64L1 10m22 4l-4.64 4.36A9 9 0 0 1 3.51 15" stroke-linecap="round" stroke-linejoin="round"/></svg>
                            {/if}
                            Retry
                          </Button>
                        </div>

                      <!-- Archived (done): sizes + timestamp + reveal -->
                      {:else if stage === 'done'}
                        <div class="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] tabular-nums text-[var(--color-muted)]">
                          {#if ep.source_size != null && encoded.length}
                            {@const encTotal = encoded.reduce((n, o) => n + (o.encoded_size ?? 0), 0)}
                            <span>{formatBytes(ep.source_size)} <span class="text-[var(--color-faint)]">→</span> {formatBytes(encTotal)}</span>
                          {:else if encoded.length}
                            <span>{formatBytes(encoded.reduce((n, o) => n + (o.encoded_size ?? 0), 0))}</span>
                          {/if}
                          {#if ep.encoded_at}
                            <span class="text-[var(--color-faint)]">{formatDate(ep.encoded_at)}</span>
                          {/if}
                          {#if ep.outputs.length}
                            <span class="flex flex-wrap gap-1">
                              {#each ep.outputs as out (out.id)}
                                <span class="bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] font-medium ring-1 ring-[var(--color-border)] {statusColor(out.status)}">{out.resolution}p</span>
                              {/each}
                            </span>
                          {/if}
                          <Button class="ml-auto" size="sm" variant="secondary" onclick={() => reveal(ep)} disabled={epBusy.has(ep.id)}>
                            {#if epBusy.has(ep.id)}<Spinner size={11} />{:else}
                              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z" stroke-linecap="round" stroke-linejoin="round"/></svg>
                            {/if}
                            Reveal
                          </Button>
                        </div>

                      <!-- Queued / other passive states -->
                      {:else}
                        <div class="mt-1 flex items-center gap-3 text-[11px] text-[var(--color-muted)]">
                          {#if ep.published_at}
                            <span>{formatDate(ep.published_at)}</span>
                          {/if}
                          {#if ep.source_size != null}
                            <span class="tabular-nums">{formatBytes(ep.source_size)}</span>
                          {/if}
                        </div>
                      {/if}
                    </div>
                  </div>
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {/each}
    </div>
  {/if}
</div>
