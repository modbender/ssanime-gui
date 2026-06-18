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
  import Spinner from '$lib/components/Spinner.svelte'
  import Button from '$lib/components/Button.svelte'
  import { markTracked, trackedAnilistIds } from '$lib/discovery.svelte'
  import { errMessage, watchBucket } from '$lib/utils'
  import { requireSource } from '$lib/sources.svelte'
  import { sseState } from '$lib/sse.svelte'

  let rows = $state<DiscoveryRow[]>([])
  let heroItems = $state<DiscoveryItem[]>([])
  let activitySeries = $state<ActivitySeries[]>([])
  let watching = $state<SeriesProgress[]>([])
  let discoveryLoading = $state(true)
  let discoveryFailed = $state(false)
  let trackingId = $state<number | null>(null)

  // Hero pulls a fresh random lineup from the top of the trending row each open.
  // The pool is capped to the items the server enriches with logos + wide art.
  const HERO_POOL_SIZE = 12
  const HERO_COUNT = 6

  // Episode statuses that count as "in the active pipeline" for the
  // Currently-downloading row (a series shows once any episode is mid-flight).
  const ACTIVE_PIPELINE = new Set([
    'queued', 'downloading', 'downloaded', 'encoding', 'encoded',
  ])

  // Number of placeholder rows to show while the discovery cache is cold.
  const SKELETON_ROWS = ['Trending now', 'Popular this season', 'All-time popular', 'Action']

  // The server discovery cache warms asynchronously after the daemon boots, so a
  // cold first call returns empty rows. Poll until rows arrive instead of
  // stranding the page on a one-shot empty result; give up after ~36s with a
  // retry affordance rather than spinning forever.
  async function loadDiscovery() {
    discoveryLoading = true
    discoveryFailed = false
    for (let attempt = 0; attempt < 24; attempt++) {
      try {
        const res = await api.getDiscovery()
        const got = (res.rows ?? []).filter((r) => r.items && r.items.length > 0)
        if (got.length > 0) {
          rows = got
          pickHero()
          discoveryLoading = false
          return
        }
      } catch {
        // Transient during boot — keep retrying.
      }
      await new Promise((r) => setTimeout(r, 1500))
    }
    discoveryLoading = false
    discoveryFailed = true
  }

  // Fresh random hero lineup from the enriched top of the trending row.
  function pickHero() {
    const trending = rows.find((r) => r.key === 'trending') ?? rows[0]
    const pool = (trending?.items ?? []).slice(0, HERO_POOL_SIZE)
    const a = [...pool]
    for (let i = a.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1))
      ;[a[i], a[j]] = [a[j], a[i]]
    }
    heroItems = a.slice(0, HERO_COUNT)
  }

  // Seed the tracked-set so discovery cards/Hero paint as "Subscribed". Authoritative:
  // rebuilt from the server each load so an unsubscribed series drops back to
  // "Subscribe" instead of lingering. Only actively-subscribed rows count — an
  // unsubscribed-but-downloaded series (Library "Downloaded") must NOT show tracked.
  async function loadTrackedSeed() {
    try {
      const res = await api.getTracked()
      const all = [
        ...(res.in_progress ?? []),
        ...(res.completed ?? []),
        ...(res.paused ?? []),
        ...(res.dropped ?? []),
      ]
      // Continue-watching row: subscribed series in the "watching" bucket (same
      // rule Library uses), regardless of whether anything is downloading now.
      watching = all.filter((s) => watchBucket(s) === 'watching')
      const next = new Set<number>()
      for (const s of all) {
        if (s.subscribed && s.anilist_id != null) next.add(s.anilist_id)
      }
      // Rebuild in place: drop stale ids, keep any optimistic add from an in-flight
      // subscribe (its id is already in `next` if the server has it, else re-added
      // by markTracked on the user action).
      for (const id of trackedAnilistIds) {
        if (!next.has(id)) trackedAnilistIds.delete(id)
      }
      for (const id of next) trackedAnilistIds.add(id)
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

  // ---- Subscribe (optimistic) ----
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
    } catch (e: unknown) {
      // 409 / already-tracked: backend may surface this as an error string.
      // Treat "already" as success; otherwise roll back the optimistic flag.
      const msg = errMessage(e).toLowerCase()
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

<div class="relative flex flex-col h-full overflow-y-auto">
  <!-- Warm-up overlay: blurs the skeletons loading behind it until discovery lands. -->
  {#if discoveryLoading && rows.length === 0}
    <div class="absolute inset-0 z-20 flex items-center justify-center bg-[var(--color-bg)]/40 backdrop-blur-md">
      <div class="flex flex-col items-center gap-4 px-6 text-center">
        <Spinner size={38} />
        <div class="space-y-1">
          <p class="text-sm font-medium text-[var(--color-text)]">Preparing your discovery feed…</p>
          <p class="text-xs text-[var(--color-muted)]">Pulling trending &amp; popular titles — this takes a moment on a fresh start.</p>
        </div>
      </div>
    </div>
  {/if}

  <!-- HERO -->
  {#if discoveryLoading && heroItems.length === 0}
    <div class="relative min-h-[72vh] -ml-[var(--rail)] w-[calc(100%+var(--rail))] shrink-0 overflow-hidden">
      <div class="absolute inset-0 bg-gradient-to-br from-white/[0.04] to-transparent animate-pulse"></div>
      <div class="absolute inset-0 bg-gradient-to-t from-[var(--color-bg)] via-[var(--color-bg)]/50 to-transparent"></div>
      <div class="relative w-full pr-6 sm:pr-10 pl-[calc(var(--rail)+1.5rem)] sm:pl-[calc(var(--rail)+2.5rem)] pt-32 pb-12 space-y-4">
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
    <!-- CONTINUE WATCHING — subscribed "watching" series, regardless of download state. -->
    {#if watching.length > 0}
      <Carousel title="Continue watching" count={watching.length}>
        {#each watching as s (s.id)}
          <div class="snap-start shrink-0 w-[150px]">
            <PosterCard series={s} />
          </div>
        {/each}
      </Carousel>
    {/if}

    <!-- CURRENTLY DOWNLOADING — shown only when something is actually in flight. -->
    {#if inProgress.length > 0}
      <Carousel title="Currently downloading" count={inProgress.length}>
        {#each inProgress as s (s.id)}
          <div class="snap-start shrink-0 w-[150px]">
            <PosterCard series={s} showProgress />
          </div>
        {/each}
      </Carousel>
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

      {#if discoveryFailed && rows.length === 0}
        <!-- Polled out: daemon still warming or unreachable. Offer a manual retry. -->
        <div class="flex flex-col items-center justify-center gap-4 py-24 text-center">
          <div class="w-14 h-14 bg-white/[0.04] ring-1 ring-white/10 flex items-center justify-center text-[var(--color-faint)]">
            <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.25"><path d="M12 3v12m0 0 4-4m-4 4-4-4M5 21h14" stroke-linecap="round" stroke-linejoin="round"/></svg>
          </div>
          <div class="space-y-1.5">
            <h2 class="text-base font-semibold tracking-tight">Discovery didn't load</h2>
            <p class="max-w-sm text-sm text-[var(--color-muted)]">The daemon may still be warming up or briefly unreachable.</p>
          </div>
          <Button onclick={loadDiscovery}>Retry</Button>
        </div>
      {/if}
    {/if}
  </div>
</div>
