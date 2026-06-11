<script lang="ts">
  import { navigate } from 'svelte-routing'
  import {
    api,
    type SeriesDetail as SeriesDetailType,
    type EpisodeDetail,
    type Profile,
    type DiscoveryItem,
    type AvailableEpisode,
  } from '$lib/api'
  import Button from '$lib/components/Button.svelte'
  import Modal from '$lib/components/Modal.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import ProgressBar from '$lib/components/ProgressBar.svelte'
  import {
    statusColor,
    derivedStatusColor,
    derivedStatusLabel,
    formatBytes,
    resolveAccent,
    hexToRgbChannels,
    trackedStatus,
    titleCase,
  } from '$lib/utils'
  import { sseState } from '$lib/sse.svelte'
  import { getPreview, markTracked } from '$lib/discovery.svelte'

  let { id, anilistId }: { id?: string; anilistId?: string } = $props()

  // Two entry modes off one page:
  //   /series/:id              → tracked series (DB id)
  //   /series/anilist/:anilist → untracked discovery preview (no DB row yet)
  const numId = $derived(id ? Number(id) : null)
  const numAnilist = $derived(anilistId ? Number(anilistId) : null)

  // ---- Tracked-series state ----
  let series = $state<SeriesDetailType | null>(null)
  let profiles = $state<Profile[]>([])
  let loading = $state(true)
  let error = $state('')

  // ---- Untracked preview ----
  let preview = $state<DiscoveryItem | null>(null)
  let tracking = $state(false)

  // ---- Manual status actions ----
  let statusBusy = $state(false)

  // ---- Available episodes (on-demand source check) ----
  let available = $state<AvailableEpisode[]>([])
  let availableLoading = $state(false)
  let availableChecked = $state(false)
  let downloadingEp = $state<string | null>(null) // keyed by source_url

  // ---- Encode modal ----
  let selected = $state<Set<number>>(new Set())
  let encodeOpen = $state(false)
  let encodeProfileId = $state<number | null>(null)
  let encodeResolutions = $state<number[]>([1080, 720])
  let encoding = $state(false)

  let scanning = $state(false)
  let refreshing = $state(false)

  async function loadTracked() {
    if (numId == null) return
    loading = true; error = ''
    try {
      [series, profiles] = await Promise.all([
        api.getSeries(numId),
        api.listProfiles(),
      ])
      encodeProfileId = series?.default_profile_id ?? null
    } catch (e: any) { error = e.message }
    finally { loading = false }
  }

  function loadPreview() {
    if (numAnilist == null) return
    loading = true; error = ''
    preview = getPreview(numAnilist)
    // No GET-by-anilist-id in the contract; the discovery cache is the source.
    if (!preview) {
      error = 'This title is no longer in the discovery cache. Open it from the home page.'
    }
    loading = false
  }

  $effect(() => {
    if (numId != null) loadTracked()
    else if (numAnilist != null) loadPreview()
  })

  // ---- Track flow (untracked → tracked) ----
  async function track(anilist: number) {
    if (tracking) return
    tracking = true
    try {
      const res = await api.trackSeries({ anilist_id: anilist })
      markTracked(anilist)
      navigate(`/series/${res.series_id}`)
    } catch (e: any) {
      const msg = String(e?.message ?? '').toLowerCase()
      if (msg.includes('already') || msg.includes('exist')) {
        // Idempotent: backend may not return the id in the error; bounce home to re-resolve.
        markTracked(anilist)
        navigate('/downloads')
      } else {
        alert(e.message)
      }
    } finally {
      tracking = false
    }
  }

  // ---- Pause / Drop / Resume ----
  async function runStatus(fn: () => Promise<{ series: any }>) {
    if (numId == null || statusBusy) return
    statusBusy = true
    try {
      await fn()
      await loadTracked()
    } catch (e: any) {
      alert(e.message)
    } finally {
      statusBusy = false
    }
  }
  const pause = () => runStatus(() => api.pauseSeries(numId!))
  const drop = () => runStatus(() => api.dropSeries(numId!))
  const resume = () => runStatus(() => api.resumeSeries(numId!))

  // ---- Available episodes ----
  async function loadAvailable() {
    if (numId == null || availableLoading) return
    availableLoading = true
    try {
      const res = await api.getAvailable(numId)
      available = res.episodes ?? []
    } catch (e: any) {
      alert(e.message || 'Source check failed.')
    } finally {
      availableLoading = false
      availableChecked = true
    }
  }

  async function downloadAvailable(ep: AvailableEpisode) {
    if (numId == null || downloadingEp) return
    downloadingEp = ep.source_url
    try {
      await api.downloadAvailable(numId, {
        source_url: ep.source_url,
        number: ep.number,
        resolution: ep.resolution || undefined,
      })
      // Manual download re-engages the series (→ Active) and pulls it into the pipeline.
      available = available.filter((e) => e.source_url !== ep.source_url)
      await loadTracked()
    } catch (e: any) {
      alert(e.message)
    } finally {
      downloadingEp = null
    }
  }

  // ---- Encode (existing) ----
  const allSelected = $derived(
    series?.episodes.length ? selected.size === series.episodes.length : false
  )
  function toggleAll() {
    if (allSelected) selected = new Set()
    else selected = new Set(series!.episodes.map((e) => e.id))
  }
  function toggleEpisode(epId: number) {
    const s = new Set(selected)
    if (s.has(epId)) s.delete(epId); else s.add(epId)
    selected = s
  }
  async function startEncode() {
    if (selected.size === 0) return
    encoding = true
    try {
      await api.bulkEncode({
        episode_ids: [...selected],
        profile_id: encodeProfileId ?? undefined,
        resolutions: encodeResolutions,
      })
      encodeOpen = false
      selected = new Set()
      await loadTracked()
    } catch (e: any) { alert(e.message) }
    finally { encoding = false }
  }

  async function refreshMeta() {
    if (numId == null) return
    refreshing = true
    try {
      await api.refreshSeries(numId)
      await loadTracked()
    } catch (e: any) {
      alert(e.message || 'AniList is rate-limited right now — existing metadata kept. Try again shortly.')
    } finally { refreshing = false }
  }

  async function scan() {
    if (numId == null) return
    scanning = true
    try {
      const eps = await api.scanEpisodes(numId)
      if (series) series = { ...series, episodes: eps }
    } catch (e: any) { alert(e.message) }
    finally { scanning = false }
  }

  const resolutionOptions = [2160, 1080, 720, 480, 360]
  function toggleResolution(res: number) {
    if (encodeResolutions.includes(res)) encodeResolutions = encodeResolutions.filter((r) => r !== res)
    else encodeResolutions = [...encodeResolutions, res]
  }

  function getEpisodeProgress(ep: EpisodeDetail) {
    return sseState.downloadProgress[ep.id] ?? sseState.encodeProgress[ep.id] ?? null
  }
  function liveStatus(ep: EpisodeDetail) {
    return sseState.episodeStatus[ep.id] ?? ep.status
  }

  // ---- Presentation (works for both modes) ----
  const accent = $derived(resolveAccent(series?.cover_color ?? preview?.cover_color))
  const accentRgb = $derived(hexToRgbChannels(series?.cover_color ?? preview?.cover_color))

  const title = $derived(
    series
      ? series.english_title || series.romaji_title || series.title
      : preview
        ? preview.english_title || preview.romaji_title
        : '',
  )
  const romaji = $derived(series?.romaji_title ?? preview?.romaji_title ?? null)
  const banner = $derived(
    series?.banner_image_url || series?.cover_image_url ||
    preview?.banner_image || preview?.cover_image || null,
  )
  const cover = $derived(series?.cover_image_url ?? preview?.cover_image ?? null)
  const format = $derived(series?.format ?? (preview ? titleCase(preview.format) : null))

  const archivedCount = $derived(
    series ? series.episodes.filter((e) => e.status === 'archived').length : 0,
  )
  const totalCount = $derived(series?.episodes.length ?? 0)

  // The displayed status honors the manual override layer.
  const status = $derived(series ? trackedStatus(series) : null)
  const isManual = $derived(status === 'paused' || status === 'dropped')

  function fmtAiring(s: string | null | undefined): string | null {
    if (!s) return null
    return titleCase(s)
  }
</script>

<div class="flex h-full flex-col overflow-y-auto">
  {#if loading}
    <div class="flex flex-1 items-center justify-center text-[var(--color-muted)]">
      <Spinner size={30} />
    </div>
  {:else if error}
    <div class="flex flex-1 flex-col items-center justify-center gap-4 text-center px-6">
      <p class="text-sm text-[var(--color-error)]">{error}</p>
      <Button variant="outline" onclick={() => navigate('/')}>Back to home</Button>
    </div>
  {:else if series || preview}
    <!-- ─── Cinematic header ─────────────────────────────────────── -->
    <section
      class="relative w-full shrink-0 overflow-hidden"
      style="--accent: {accent}; --accent-rgb: {accentRgb};"
    >
      <!-- Banner layer -->
      <div class="absolute inset-0">
        {#if banner}
          {#key banner}
            <img
              src={banner}
              alt=""
              class="h-full w-full object-cover object-center animate-fade"
              style="animation-duration:.9s"
            />
          {/key}
        {:else}
          <div
            class="h-full w-full"
            style="background:
              radial-gradient(120% 100% at 80% 0%, rgb(var(--accent-rgb) / 0.35), transparent 60%),
              radial-gradient(100% 100% at 0% 100%, rgb(var(--accent-rgb) / 0.18), transparent 55%),
              var(--color-surface);"
          ></div>
        {/if}
      </div>

      <!-- Scrims -->
      <div class="absolute inset-0 bg-gradient-to-t from-[var(--color-bg)] via-[var(--color-bg)]/55 to-transparent"></div>
      <div class="absolute inset-0 bg-gradient-to-r from-[var(--color-bg)] via-[var(--color-bg)]/60 to-transparent"></div>
      <div
        class="absolute inset-0 mix-blend-soft-light opacity-60"
        style="background: radial-gradient(90% 120% at 10% 100%, rgb(var(--accent-rgb) / 0.55), transparent 60%);"
      ></div>

      <!-- Back affordance -->
      <div class="relative px-6 sm:px-10 pt-5">
        <Button variant="ghost" size="sm" onclick={() => navigate(series ? '/downloads' : '/')}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M19 12H5M12 5l-7 7 7 7" stroke-linecap="round" stroke-linejoin="round"/></svg>
          {series ? 'Downloads' : 'Home'}
        </Button>
      </div>

      <!-- Content -->
      <div class="relative px-6 sm:px-10 pt-10 pb-9 sm:pt-16 sm:pb-12">
        <div class="flex flex-col gap-6 sm:flex-row sm:items-end sm:gap-7 animate-fade-up">
          <!-- Poster -->
          <div class="shrink-0">
            <div class="w-32 sm:w-44 overflow-hidden rounded-[var(--radius-card)] ring-1 ring-white/10 shadow-[0_24px_60px_-20px_rgba(0,0,0,0.85)]">
              {#if cover}
                <img src={cover} alt={title} class="aspect-[2/3] w-full object-cover" />
              {:else}
                <div
                  class="aspect-[2/3] w-full"
                  style="background: radial-gradient(120% 120% at 50% 0%, rgb(var(--accent-rgb) / 0.3), var(--color-surface-2));"
                ></div>
              {/if}
            </div>
          </div>

          <!-- Meta -->
          <div class="min-w-0 flex-1">
            <!-- eyebrow -->
            <div class="mb-3 flex items-center gap-2">
              {#if series}
                <span class="inline-flex items-center gap-1.5 rounded-full bg-white/5 px-3 py-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-[var(--color-text-dim)] ring-1 ring-white/10">
                  <span class="h-1.5 w-1.5 rounded-full bg-[var(--accent)]"></span>
                  Season {series.season_number}
                </span>
                {#if series.feed_title}
                  <span class="text-[11px] text-[var(--color-muted)]">via {series.feed_title}</span>
                {/if}
              {:else}
                <span class="inline-flex items-center gap-1.5 rounded-full bg-white/5 px-3 py-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-[var(--color-text-dim)] ring-1 ring-white/10">
                  <span class="h-1.5 w-1.5 rounded-full bg-[var(--accent)]"></span>
                  Discovery
                </span>
              {/if}
            </div>

            <!-- title -->
            <h1 class="text-3xl sm:text-4xl lg:text-5xl font-extrabold leading-[1.05] tracking-tight text-white drop-shadow-[0_2px_20px_rgba(0,0,0,0.6)]">
              {title}
            </h1>
            {#if romaji && romaji !== title}
              <p class="mt-1.5 text-sm text-[var(--color-text-dim)]">{romaji}</p>
            {/if}

            <!-- meta chips -->
            <div class="mt-5 flex flex-wrap items-center gap-2">
              {#if series && status}
                <span class={`inline-flex items-center rounded-full border px-2.5 py-1 text-[11px] font-medium backdrop-blur-sm ${derivedStatusColor(status)}`}>
                  {derivedStatusLabel(status)}
                </span>
              {/if}
              {#if format}
                <span class="inline-flex items-center rounded-full bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)] ring-1 ring-white/10">{format}</span>
              {/if}
              {#if series && fmtAiring(series.airing_status)}
                <span class="inline-flex items-center rounded-full bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)] ring-1 ring-white/10">{fmtAiring(series.airing_status)}</span>
              {:else if preview && preview.status}
                <span class="inline-flex items-center rounded-full bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)] ring-1 ring-white/10">{titleCase(preview.status)}</span>
              {/if}
              {#if (series?.episode_count ?? preview?.episode_count)}
                <span class="inline-flex items-center rounded-full bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)] ring-1 ring-white/10">{series?.episode_count ?? preview?.episode_count} episodes</span>
              {/if}
              {#if series}
                <span class="inline-flex items-center gap-1.5 rounded-full bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium tabular-nums text-[var(--color-text-dim)] ring-1 ring-white/10">
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 9h18" stroke-linecap="round"/></svg>
                  {archivedCount}/{totalCount} archived
                </span>
              {:else if preview && preview.season_year}
                <span class="inline-flex items-center rounded-full bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)] ring-1 ring-white/10">{titleCase(preview.season)} {preview.season_year}</span>
              {/if}
            </div>

            <!-- actions -->
            <div class="mt-6 flex flex-wrap items-center gap-2">
              {#if preview && !series}
                <!-- Untracked: the single tracked-series creation path -->
                <Button onclick={() => track(numAnilist!)} disabled={tracking}>
                  {#if tracking}
                    <Spinner size={14}/>
                    Tracking…
                  {:else}
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 3v12m0 0 4-4m-4 4-4-4M5 21h14" stroke-linecap="round" stroke-linejoin="round"/></svg>
                    Download &amp; track
                  {/if}
                </Button>
              {:else if series}
                <!-- Tracked: manual status controls -->
                {#if isManual}
                  <Button onclick={resume} disabled={statusBusy}>
                    {#if statusBusy}<Spinner size={14}/>{:else}
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" stroke="none"><path d="M8 5v14l11-7z"/></svg>
                    {/if}
                    Resume
                  </Button>
                {:else}
                  <Button variant="outline" onclick={pause} disabled={statusBusy} title="Pause auto-download (keeps files)">
                    {#if statusBusy}<Spinner size={14}/>{:else}
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25"><path d="M6 4h3v16H6zM15 4h3v16h-3z"/></svg>
                    {/if}
                    Pause
                  </Button>
                {/if}
                {#if status !== 'dropped'}
                  <Button variant="ghost" onclick={drop} disabled={statusBusy} title="Drop (keeps files on disk)">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6 6 18M6 6l12 12" stroke-linecap="round"/></svg>
                    Drop
                  </Button>
                {/if}

                <Button variant="secondary" onclick={scan} disabled={scanning}>
                  {#if scanning}<Spinner size={14}/>{:else}
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 21l-6-6m2-5a7 7 0 1 1-14 0 7 7 0 0 1 14 0z" stroke-linecap="round" stroke-linejoin="round"/></svg>
                  {/if}
                  Scan torrents
                </Button>

                <Button variant="ghost" onclick={refreshMeta} disabled={refreshing} title="Refresh AniList metadata">
                  {#if refreshing}<Spinner size={14}/>{:else}
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M23 4v6h-6M1 20v-6h6" stroke-linecap="round" stroke-linejoin="round"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" stroke-linecap="round" stroke-linejoin="round"/></svg>
                  {/if}
                  Refresh
                </Button>

                {#if selected.size > 0}
                  <Button onclick={() => { encodeOpen = true }}>
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 3v18l15-9z" fill="currentColor" stroke="none"/></svg>
                    Download &amp; Encode ({selected.size})
                  </Button>
                {/if}
              {/if}
            </div>

            {#if series && isManual}
              <p class="mt-3 text-xs text-[var(--color-muted)] max-w-xl">
                {status === 'paused' ? 'Paused' : 'Dropped'} — background auto-download is off. Files are kept. Downloading an episode below re-engages tracking.
              </p>
            {/if}
          </div>
        </div>
      </div>
    </section>

    {#if series}
      <!-- ─── Available episodes (on-demand source check) ──────────── -->
      <section class="px-6 sm:px-10 pt-2">
        <div class="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--color-border)] bg-[var(--color-surface)]">
          <div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3 sm:px-5">
            <span class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Available at source</span>
            {#if availableChecked}
              <span class="rounded-full bg-white/[0.06] px-2 py-0.5 text-[10px] font-semibold tabular-nums text-[var(--color-text-dim)]">{available.length}</span>
            {/if}
            <Button variant="ghost" size="sm" class="ml-auto" onclick={loadAvailable} disabled={availableLoading}>
              {#if availableLoading}<Spinner size={13}/>{:else}
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 21l-6-6m2-5a7 7 0 1 1-14 0 7 7 0 0 1 14 0z" stroke-linecap="round" stroke-linejoin="round"/></svg>
              {/if}
              {availableChecked ? 'Re-check source' : 'Check source'}
            </Button>
          </div>

          {#if !availableChecked}
            <p class="px-5 py-6 text-sm text-[var(--color-muted)]">Run a one-time source check to list episodes available but not yet downloaded — works even while paused or dropped.</p>
          {:else if available.length === 0}
            <p class="px-5 py-6 text-sm text-[var(--color-muted)]">No new episodes at the source right now.</p>
          {:else}
            <ul class="divide-y divide-[var(--color-border)]/60">
              {#each available as ep (ep.source_url)}
                <li class="flex items-center gap-3 px-4 py-3 sm:px-5">
                  <span class="shrink-0 rounded-lg bg-[var(--color-surface-2)] px-2 py-1 text-[11px] font-semibold tabular-nums text-[var(--color-text-dim)] ring-1 ring-[var(--color-border)]">
                    E{String(ep.number).padStart(2, '0')}
                  </span>
                  <div class="min-w-0 flex-1">
                    <span class="block truncate text-sm text-[var(--color-text)]">{ep.title || `Episode ${ep.number}`}</span>
                    <span class="text-xs text-[var(--color-muted)] tabular-nums">
                      {ep.resolution}{#if ep.size} · {formatBytes(ep.size)}{/if}
                    </span>
                  </div>
                  <Button
                    size="sm"
                    onclick={() => downloadAvailable(ep)}
                    disabled={downloadingEp === ep.source_url}
                  >
                    {#if downloadingEp === ep.source_url}<Spinner size={12}/>{:else}
                      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25"><path d="M12 3v12m0 0 4-4m-4 4-4-4M5 21h14" stroke-linecap="round" stroke-linejoin="round"/></svg>
                    {/if}
                    Download
                  </Button>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      </section>

      <!-- ─── Episode list (pipeline) ──────────────────────────────── -->
      <section class="px-6 sm:px-10 pt-8 pb-16">
        <div class="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--color-border)] bg-[var(--color-surface)]">
          <div class="sticky top-0 z-10 flex items-center gap-3 border-b border-[var(--color-border)] bg-[var(--color-surface)]/95 px-4 py-3 backdrop-blur-md sm:px-5">
            <input
              type="checkbox"
              checked={allSelected}
              onchange={toggleAll}
              class="h-4 w-4 shrink-0 cursor-pointer rounded border-[var(--color-border-strong)] accent-[var(--accent)]"
              aria-label="Select all episodes"
            />
            <span class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Episode</span>
            {#if selected.size > 0}
              <span class="rounded-full bg-[rgb(var(--accent-rgb)/0.16)] px-2 py-0.5 text-[10px] font-semibold tabular-nums text-[var(--accent)]">{selected.size} selected</span>
            {/if}
            <span class="ml-auto hidden text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)] sm:inline">Status</span>
            <span class="hidden w-28 text-right text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)] sm:inline">Outputs</span>
          </div>

          {#if series.episodes.length === 0}
            <div class="flex flex-col items-center justify-center gap-4 px-6 py-20 text-center">
              <div class="flex h-14 w-14 items-center justify-center rounded-2xl bg-white/[0.04] text-[var(--color-faint)] ring-1 ring-white/10">
                <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.25"><path d="M9 12h6m-6 4h6m2 5H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5.586a1 1 0 0 1 .707.293l5.414 5.414a1 1 0 0 1 .293.707V19a2 2 0 0 1-2 2z" stroke-linecap="round" stroke-linejoin="round"/></svg>
              </div>
              <div class="space-y-1.5">
                <h2 class="text-base font-semibold tracking-tight">No episodes downloaded yet</h2>
                <p class="max-w-sm text-sm text-[var(--color-muted)]">The auto-downloader will grab new episodes as they air. Or check the source above and download one now.</p>
              </div>
            </div>
          {:else}
            <ul class="divide-y divide-[var(--color-border)]/60">
              {#each series.episodes as ep (ep.id)}
                {@const progress = getEpisodeProgress(ep)}
                {@const epStatus = liveStatus(ep)}
                {@const isSelected = selected.has(ep.id)}
                <li
                  class="group flex items-center gap-3 px-4 py-3 transition-colors duration-200 sm:px-5
                    {isSelected ? 'bg-[rgb(var(--accent-rgb)/0.07)]' : 'hover:bg-white/[0.025]'}"
                >
                  <input
                    type="checkbox"
                    checked={isSelected}
                    onchange={() => toggleEpisode(ep.id)}
                    class="h-4 w-4 shrink-0 cursor-pointer rounded border-[var(--color-border-strong)] accent-[var(--accent)]"
                    aria-label={`Select episode ${ep.episode_no ?? ep.title ?? ep.id}`}
                  />
                  <span class="shrink-0 rounded-lg bg-[var(--color-surface-2)] px-2 py-1 text-[11px] font-semibold tabular-nums text-[var(--color-text-dim)] ring-1 ring-[var(--color-border)]">
                    {ep.episode_no != null ? `E${String(ep.episode_no).padStart(2, '0')}` : 'SP'}
                  </span>
                  <div class="min-w-0 flex-1">
                    <div class="flex items-baseline gap-2">
                      <span class="truncate text-sm text-[var(--color-text)]">{ep.title ?? `Episode ${ep.episode_no ?? ''}`}</span>
                      {#if ep.release_group}
                        <span class="hidden shrink-0 text-xs text-[var(--color-muted)] sm:inline">[{ep.release_group}]</span>
                      {/if}
                      {#if ep.source_size}
                        <span class="hidden shrink-0 text-xs tabular-nums text-[var(--color-faint)] sm:inline">{formatBytes(ep.source_size)}</span>
                      {/if}
                    </div>
                    {#if progress && 'percent' in progress}
                      <div class="mt-2 flex items-center gap-2">
                        <ProgressBar value={progress.percent} max={100} class="flex-1" />
                        <span class="shrink-0 text-[10px] font-medium tabular-nums text-[var(--color-muted)]">{Math.round(progress.percent)}%</span>
                      </div>
                    {/if}
                    {#if epStatus === 'error' && ep.error_message}
                      <p class="mt-1 truncate text-xs text-[var(--color-error)]" title={ep.error_message}>{ep.error_message}</p>
                    {/if}
                  </div>
                  <span class="shrink-0 text-xs font-medium {statusColor(epStatus)}">{epStatus}</span>
                  <div class="hidden w-28 flex-wrap justify-end gap-1 sm:flex">
                    {#each ep.outputs as out (out.id)}
                      <span
                        class="rounded-md bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[11px] font-medium tabular-nums ring-1 ring-[var(--color-border)] {statusColor(out.status)}"
                        title={out.error_message ?? out.status}
                      >
                        {out.resolution}p
                      </span>
                    {/each}
                  </div>
                  <div class="flex w-7 shrink-0 justify-end">
                    {#if epStatus === 'error'}
                      <Button
                        variant="ghost"
                        size="icon"
                        class="h-7 w-7"
                        onclick={() => api.retryEpisode(ep.id).then(() => loadTracked())}
                        title="Retry episode"
                      >
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25"><path d="M1 4v6h6M23 20v-6h-6" stroke-linecap="round" stroke-linejoin="round"/><path d="M20.49 9A9 9 0 0 0 5.64 5.64L1 10m22 4l-4.64 4.36A9 9 0 0 1 3.51 15" stroke-linecap="round" stroke-linejoin="round"/></svg>
                      </Button>
                    {/if}
                  </div>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      </section>
    {:else if preview}
      <!-- Untracked: a short call-to-action panel under the hero -->
      <section class="px-6 sm:px-10 pb-16">
        <div class="rounded-[var(--radius-lg)] border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-8 text-center sm:px-8">
          <p class="mx-auto max-w-md text-sm text-[var(--color-muted)]">
            Not tracked yet. Hit <span class="font-medium text-[var(--color-text)]">Download &amp; track</span> to create the series, subscribe to its feed, and let the pipeline auto-fetch and re-encode every episode.
          </p>
        </div>
      </section>
    {/if}
  {/if}
</div>

<!-- ─── Encode modal ───────────────────────────────────────────── -->
<Modal bind:open={encodeOpen} title="Download & Encode {selected.size} episode{selected.size === 1 ? '' : 's'}">
  {#snippet footer()}
    <Button variant="ghost" onclick={() => { encodeOpen = false }}>Cancel</Button>
    <Button onclick={startEncode} disabled={encoding || encodeResolutions.length === 0}>
      {#if encoding}<Spinner size={14}/>{/if}
      Start
    </Button>
  {/snippet}

  <div class="space-y-5">
    <div>
      <label for="encode-profile" class="mb-2 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Encode profile</label>
      <select
        id="encode-profile"
        bind:value={encodeProfileId}
        class="h-9 w-full cursor-pointer rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] transition-colors focus:border-[var(--accent)] focus:outline-none"
      >
        <option value={null}>Default profile</option>
        {#each profiles as p (p.id)}
          <option value={p.id}>{p.name}{p.is_builtin ? ' (builtin)' : ''}</option>
        {/each}
      </select>
    </div>

    <div>
      <span class="mb-2 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Output resolutions</span>
      <div class="flex flex-wrap gap-2">
        {#each resolutionOptions as res}
          {@const on = encodeResolutions.includes(res)}
          <button
            type="button"
            aria-pressed={on}
            class="rounded-xl border px-3.5 py-1.5 text-sm font-medium transition-colors duration-200
              {on
                ? 'border-[var(--accent)] bg-[rgb(var(--accent-rgb)/0.15)] text-[var(--color-text)]'
                : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:border-[var(--color-border-strong)] hover:text-[var(--color-text)]'}"
            onclick={() => toggleResolution(res)}
          >{res}p</button>
        {/each}
      </div>
      {#if encodeResolutions.length === 0}
        <p class="mt-2 text-xs text-[var(--color-warning)]">Select at least one resolution.</p>
      {/if}
    </div>
  </div>
</Modal>
