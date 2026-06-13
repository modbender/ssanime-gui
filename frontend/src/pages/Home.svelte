<script lang="ts">
  import {
    api,
    type DiscoveryRow,
    type DiscoveryItem,
    type SeriesProgress,
    type ActivitySeries,
  } from '$lib/api'
  import Hero from '$lib/components/Hero.svelte'
  import Carousel from '$lib/components/Carousel.svelte'
  import CarouselSkeleton from '$lib/components/CarouselSkeleton.svelte'
  import PosterCard from '$lib/components/PosterCard.svelte'
  import DiscoveryCard from '$lib/components/DiscoveryCard.svelte'
  import { markTracked, trackedAnilistIds } from '$lib/discovery.svelte'
  import { requireSource } from '$lib/sources.svelte'
  import { sseState } from '$lib/sse.svelte'

  let rows = $state<DiscoveryRow[]>([])
  let activitySeries = $state<ActivitySeries[]>([])
  let discoveryLoading = $state(true)
  let trackingId = $state<number | null>(null)

  // Episode statuses that count as "in the active pipeline" for the
  // Currently-downloading row (a series shows once any episode is mid-flight).
  const ACTIVE_PIPELINE = new Set([
    'queued', 'downloading', 'downloaded', 'encoding', 'encoded',
  ])

  // Number of placeholder rows to show while the discovery cache is cold.
  const SKELETON_ROWS = ['Trending now', 'Popular this season', 'All-time popular', 'Action']

  async function loadDiscovery() {
    discoveryLoading = true
    try {
      const res = await api.getDiscovery()
      // Keep only rows that actually have items — empty rows are hidden.
      rows = (res.rows ?? []).filter((r) => r.items && r.items.length > 0)
    } catch {
      rows = []
    } finally {
      discoveryLoading = false
    }
  }

  // Seed the optimistic tracked-set so discovery cards paint as tracked.
  async function loadTrackedSeed() {
    try {
      const res = await api.getTracked()
      for (const s of [
        ...(res.in_progress ?? []),
        ...(res.completed ?? []),
        ...(res.paused ?? []),
        ...(res.dropped ?? []),
      ]) {
        if (s.anilist_id != null) trackedAnilistIds.add(s.anilist_id)
      }
    } catch {
      // Non-fatal: discovery still works without optimistic tracked flags.
    }
  }

  // Currently-downloading row: any series (subscribed OR manually-downloaded)
  // with at least one episode in the active pipeline. Sourced from /api/activity,
  // which now includes unsubscribed-but-has-episodes series.
  let activityInFlight = false
  async function refreshActivity() {
    if (activityInFlight) return
    activityInFlight = true
    try {
      const res = await api.getActivity()
      activitySeries = res.series ?? []
    } catch {
      // Transient daemon hiccup — keep the last snapshot; SSE keeps it live.
    } finally {
      activityInFlight = false
    }
  }

  // Live effective status (SSE override over the snapshot) drives membership so a
  // freshly-finished series drops out without a refetch.
  const inProgress = $derived(
    activitySeries.filter((s) =>
      (s.episodes ?? []).some((ep) =>
        ACTIVE_PIPELINE.has(sseState.episodeStatus[ep.id] ?? ep.status),
      ),
    ),
  )

  $effect(() => {
    loadDiscovery()
    loadTrackedSeed()
    refreshActivity()
  })

  // A status transition changes which series are active; refetch on SSE churn.
  let lastSignal = -1
  $effect(() => {
    const signal = Object.values(sseState.episodeStatus).join('|').length
    if (signal === lastSignal) return
    lastSignal = signal
    refreshActivity()
  })

  // The hero pulls from the trending row (top items, rotating).
  const heroItems = $derived.by(() => {
    const trending = rows.find((r) => r.key === 'trending') ?? rows[0]
    return (trending?.items ?? []).slice(0, 6)
  })

  // ---- Download & track (optimistic) ----
  async function track(item: DiscoveryItem) {
    if (trackingId != null) return
    if (!requireSource()) return
    trackingId = item.anilist_id
    // Optimistic: flip the card to tracked immediately.
    markTracked(item.anilist_id)
    try {
      await api.trackSeries({ anilist_id: item.anilist_id })
      // Pull the now-tracked series into the activity-backed downloading row.
      refreshActivity()
    } catch (e: any) {
      // 409 / already-tracked: backend may surface this as an error string.
      // Treat "already" as success; otherwise roll back the optimistic flag.
      const msg = String(e?.message ?? '').toLowerCase()
      if (!msg.includes('already') && !msg.includes('exist')) {
        trackedAnilistIds.delete(item.anilist_id)
      }
      // Refresh either way so the row reflects reality.
      refreshActivity()
    } finally {
      trackingId = null
    }
  }
</script>

<div class="flex flex-col h-full overflow-y-auto">
  <!-- HERO -->
  {#if discoveryLoading && heroItems.length === 0}
    <div class="relative min-h-[72vh] w-full shrink-0 overflow-hidden">
      <div class="absolute inset-0 bg-gradient-to-br from-white/[0.04] to-transparent animate-pulse"></div>
      <div class="absolute inset-0 bg-gradient-to-t from-[var(--color-bg)] via-[var(--color-bg)]/50 to-transparent"></div>
      <div class="relative w-full px-6 sm:px-10 pt-32 pb-12 space-y-4">
        <div class="h-3 w-28 bg-white/[0.06] animate-pulse"></div>
        <div class="h-12 w-2/3 max-w-xl bg-white/[0.06] animate-pulse"></div>
        <div class="h-4 w-1/2 max-w-md bg-white/[0.05] animate-pulse"></div>
        <div class="h-11 w-44 bg-white/[0.06] animate-pulse"></div>
      </div>
    </div>
  {:else if heroItems.length > 0}
    <Hero items={heroItems} onTrack={track} {trackingId} />
  {/if}

  <div class="px-6 sm:px-10 pb-16 -mt-2 space-y-12">
    <!-- CURRENTLY DOWNLOADING — always present (mirror of a persistent row);
         shows a tasteful idle state instead of a bare empty carousel. -->
    {#if inProgress.length > 0}
      <Carousel title="Currently downloading" count={inProgress.length}>
        {#each inProgress as s (s.id)}
          <div class="snap-start shrink-0 w-[150px]">
            <PosterCard series={s} showProgress />
          </div>
        {/each}
      </Carousel>
    {:else}
      <section>
        <h2 class="mb-3 text-[15px] font-semibold tracking-tight text-[var(--color-text)]">Currently downloading</h2>
        <div class="flex items-center gap-4 border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-6">
          <div class="flex h-11 w-11 shrink-0 items-center justify-center bg-white/[0.04] text-[var(--color-faint)] ring-1 ring-white/10">
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M12 3v12m0 0 4-4m-4 4-4-4M5 21h14" stroke-linecap="round" stroke-linejoin="round"/></svg>
          </div>
          <p class="text-sm text-[var(--color-muted)]">
            Nothing downloading — subscribe to a series or grab an episode from its page.
          </p>
        </div>
      </section>
    {/if}

    <!-- DISCOVERY ROWS -->
    {#if discoveryLoading && rows.length === 0}
      {#each SKELETON_ROWS as t (t)}
        <CarouselSkeleton title={t} />
      {/each}
    {:else}
      {#each rows as row (row.key)}
        <Carousel title={row.title} count={row.items.length}>
          {#each row.items as item (item.anilist_id)}
            <div class="snap-start shrink-0 w-[150px]">
              <DiscoveryCard
                {item}
                onTrack={track}
                tracking={trackingId === item.anilist_id}
              />
            </div>
          {/each}
        </Carousel>
      {/each}

      {#if !discoveryLoading && rows.length === 0}
        <!-- Discovery cache cold/unreachable AND nothing tracked: graceful, never the old empty state -->
        {#if inProgress.length === 0}
          <div class="flex flex-col items-center justify-center gap-3 py-24 text-center">
            <div class="w-14 h-14 bg-white/[0.04] ring-1 ring-white/10 flex items-center justify-center text-[var(--color-faint)]">
              <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.25"><path d="M12 3v12m0 0 4-4m-4 4-4-4M5 21h14" stroke-linecap="round" stroke-linejoin="round"/></svg>
            </div>
            <div class="space-y-1.5">
              <h2 class="text-base font-semibold tracking-tight">Warming up discovery…</h2>
              <p class="max-w-sm text-sm text-[var(--color-muted)]">Trending and popular rows populate within a few seconds of the daemon starting. Hang tight.</p>
            </div>
          </div>
        {/if}
      {/if}
    {/if}
  </div>
</div>
