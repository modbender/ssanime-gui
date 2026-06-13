<script lang="ts">
  import { navigate } from 'svelte-routing'
  import {
    api,
    type SeriesDetail as SeriesDetailType,
    type EpisodeDetail,
    type Profile,
    type DiscoveryItem,
    type AvailableEpisode,
    type AnilistDetail,
    type AnilistDetailEpisode,
    type AnilistRelatedEntry,
  } from '$lib/api'
  import Button from '$lib/components/Button.svelte'
  import Modal from '$lib/components/Modal.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import Carousel from '$lib/components/Carousel.svelte'
  import DiscoveryCard from '$lib/components/DiscoveryCard.svelte'
  import {
    statusColor,
    formatBytes,
    formatDate,
    resolveAccent,
    hexToRgbChannels,
    accentForeground,
    accentText,
    accentTextRgb,
    watchBucket,
    watchStatusColor,
    watchStatusLabel,
    titleCase,
    relativeTime,
    isFuture,
    countdown,
  } from '$lib/utils'
  import { sseState } from '$lib/sse.svelte'
  import { episodeOverall, episodeStage } from '$lib/pipeline.svelte'
  import { fillGreenMix } from '$lib/pipeline-math'
  import { getPreview, markTracked } from '$lib/discovery.svelte'
  import { requireSource } from '$lib/sources.svelte'

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

  // ---- Untracked preview (instant-paint hint only) ----
  let preview = $state<DiscoveryItem | null>(null)
  let tracking = $state(false)

  // ---- Full AniList detail (best-effort enrichment) ----
  let detail = $state<AnilistDetail | null>(null)
  let detailLoading = $state(false)

  // ---- Synopsis expand ----
  let synopsisExpanded = $state(false)

  // ---- Watch-status actions ----
  let statusBusy = $state(false)
  let unsubscribeOpen = $state(false)
  let unsubscribing = $state(false)

  // ---- Available episodes (on-demand source check) ----
  let available = $state<AvailableEpisode[]>([])
  let availableWarnings = $state<string[]>([])
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

  // The anilist id to fetch /detail for, in either mode.
  const detailAnilistId = $derived(series?.anilist_id ?? numAnilist ?? null)

  async function loadDetail(anilist: number) {
    detailLoading = true
    try {
      detail = await api.getAnilistDetail(anilist)
    } catch (e: any) {
      // Best-effort: a tracked series still renders from its pipeline rows.
      // For an untracked preview with no other data, surface the error.
      if (numId == null && !preview) error = e.message || 'Could not load this title.'
    } finally {
      detailLoading = false
    }
  }

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
    if (series?.anilist_id != null) loadDetail(series.anilist_id)
  }

  function loadPreview() {
    if (numAnilist == null) return
    loading = true; error = ''
    // Instant-paint hint from the discovery cache (optional, not required).
    preview = getPreview(numAnilist)
    loading = false
    loadDetail(numAnilist)
  }

  // Re-run whenever the route param changes (e.g. relation/recommendation nav).
  let lastKey = $state('')
  $effect(() => {
    const key = numId != null ? `id:${numId}` : numAnilist != null ? `al:${numAnilist}` : ''
    if (key === lastKey) return
    lastKey = key
    // Reset transient state so navigating between titles doesn't leak.
    detail = null; preview = null; series = null; error = ''
    available = []; availableWarnings = []; availableChecked = false; synopsisExpanded = false
    selected = new Set()
    if (numId != null) loadTracked()
    else if (numAnilist != null) loadPreview()
    else loading = false
  })

  // ---- Track flow (untracked → tracked) ----
  async function track(anilist: number) {
    if (tracking) return
    if (!requireSource()) return
    tracking = true
    try {
      const res = await api.trackSeries({ anilist_id: anilist })
      markTracked(anilist)
      navigate(`/series/${res.series_id}`)
    } catch (e: any) {
      const msg = String(e?.message ?? '').toLowerCase()
      if (msg.includes('already') || msg.includes('exist')) {
        markTracked(anilist)
        navigate('/library')
      } else {
        alert(e.message)
      }
    } finally {
      tracking = false
    }
  }

  // ---- Watch status (Watching / On Hold / Dropped) ----
  async function setStatus(status: 'watching' | 'on_hold' | 'dropped') {
    if (numId == null || statusBusy || series?.status === status) return
    statusBusy = true
    try {
      await api.setSeriesStatus(numId, status)
      await loadTracked()
    } catch (e: any) {
      alert(e.message)
    } finally {
      statusBusy = false
    }
  }

  // ---- Unsubscribe (full DB cleanup; files kept) ----
  async function unsubscribe() {
    if (numId == null || unsubscribing) return
    unsubscribing = true
    try {
      await api.unsubscribeSeries(numId)
      unsubscribeOpen = false
      navigate('/library')
    } catch (e: any) {
      alert(e.message)
    } finally {
      unsubscribing = false
    }
  }

  // ---- Available episodes ----
  async function loadAvailable() {
    if (numId == null || availableLoading) return
    availableLoading = true
    try {
      const res = await api.getAvailable(numId)
      available = res.episodes ?? []
      availableWarnings = res.warnings ?? []
    } catch (e: any) {
      alert(e.message || 'Source check failed.')
    } finally {
      availableLoading = false
      availableChecked = true
    }
  }

  async function downloadAvailable(ep: AvailableEpisode) {
    if (numId == null || downloadingEp) return
    if (!requireSource()) return
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
  const pipelineEpisodes = $derived(series?.episodes ?? [])
  const allSelected = $derived(
    pipelineEpisodes.length ? selected.size === pipelineEpisodes.length : false,
  )
  function toggleAll() {
    if (allSelected) selected = new Set()
    else selected = new Set(pipelineEpisodes.map((e) => e.id))
  }
  function toggleEpisode(epId: number) {
    const s = new Set(selected)
    if (s.has(epId)) s.delete(epId); else s.add(epId)
    selected = s
  }
  async function startEncode() {
    if (selected.size === 0) return
    if (!requireSource()) return
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
      if (series?.anilist_id != null) await loadDetail(series.anilist_id)
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
    const dl = sseState.downloadProgress[ep.id]
    if (dl) return dl
    // encodeProgress is keyed by output_id; surface the live tick for whichever
    // of this episode's outputs is currently encoding.
    for (const out of ep.outputs) {
      const enc = sseState.encodeProgress[out.id]
      if (enc) return enc
    }
    return null
  }
  function liveStatus(ep: EpisodeDetail) {
    return sseState.episodeStatus[ep.id] ?? ep.status
  }

  function openTrailer() {
    if (!detail?.trailer) return
    const vid = detail.trailer.video_id
    const url = detail.trailer.site?.toLowerCase().includes('dailymotion')
      ? `https://www.dailymotion.com/video/${vid}`
      : `https://www.youtube.com/watch?v=${vid}`
    window.open(url, '_blank', 'noopener,noreferrer')
  }

  // ---- Presentation (works for both modes) ----
  const accent = $derived(
    resolveAccent(series?.cover_color ?? preview?.cover_color ?? detail?.cover_color),
  )
  const accentRgb = $derived(
    hexToRgbChannels(series?.cover_color ?? preview?.cover_color ?? detail?.cover_color),
  )
  const accentFg = $derived(
    accentForeground(series?.cover_color ?? preview?.cover_color ?? detail?.cover_color),
  )
  const accentTxt = $derived(
    accentText(series?.cover_color ?? preview?.cover_color ?? detail?.cover_color),
  )
  const accentTxtRgb = $derived(
    accentTextRgb(series?.cover_color ?? preview?.cover_color ?? detail?.cover_color),
  )

  const title = $derived(
    series
      ? series.english_title || series.romaji_title || series.title
      : preview
        ? preview.english_title || preview.romaji_title
        : detail
          ? detail.title_english || detail.title_romaji || `AniList #${detail.anilist_id}`
          : '',
  )
  const romaji = $derived(series?.romaji_title ?? preview?.romaji_title ?? detail?.title_romaji ?? null)
  const banner = $derived(
    series?.banner_image_url || series?.cover_image_url ||
    preview?.banner_image || preview?.cover_image ||
    detail?.banner_image || detail?.cover_image || null,
  )
  const cover = $derived(series?.cover_image_url ?? preview?.cover_image ?? detail?.cover_image ?? null)
  const format = $derived(
    series?.format ??
    (preview ? titleCase(preview.format) : detail?.format ? titleCase(detail.format) : null),
  )

  const archivedCount = $derived(
    series ? series.episodes.filter((e) => e.status === 'archived').length : 0,
  )
  const totalCount = $derived(series?.episodes.length ?? 0)

  // The displayed status bucket (Completed derived, else watch status).
  const bucket = $derived(series ? watchBucket(series) : null)

  function fmtAiring(s: string | null | undefined): string | null {
    if (!s) return null
    return titleCase(s)
  }

  // ---- About strip pairs ----
  interface AboutPair { label: string; value: string }
  const aboutPairs = $derived.by<AboutPair[]>(() => {
    if (!detail) return []
    const pairs: AboutPair[] = []
    if (detail.average_score) pairs.push({ label: 'Score', value: `${detail.average_score}%` })
    if (detail.studio) pairs.push({ label: 'Studio', value: detail.studio })
    if (detail.source_material) pairs.push({ label: 'Source', value: titleCase(detail.source_material) })
    if (detail.season || detail.season_year) {
      pairs.push({ label: 'Season', value: `${titleCase(detail.season ?? '')}${detail.season && detail.season_year ? ' ' : ''}${detail.season_year ?? ''}`.trim() })
    }
    if (detail.duration_min) pairs.push({ label: 'Duration', value: `${detail.duration_min} min` })
    if (detail.episode_count) pairs.push({ label: 'Episodes', value: String(detail.episode_count) })
    if (detail.next_airing) {
      const cd = countdown(detail.next_airing.airing_at)
      if (cd) pairs.push({ label: 'Next', value: `Ep ${detail.next_airing.episode} in ${cd}` })
    }
    return pairs
  })

  // ---- Unified episode grid (merge metadata + pipeline + source) ----
  interface MergedEpisode {
    key: string
    number: number | null
    title: string | null
    thumbnail: string | null
    /** ISO date string (ani.zip) or unix-seconds (pipeline published_at). */
    airDate: string | number | null
    overview: string | null
    pipeline: EpisodeDetail | null
    source: AvailableEpisode | null
  }

  const mergedEpisodes = $derived.by<MergedEpisode[]>(() => {
    const byNumber = new Map<number, MergedEpisode>()
    const specials: MergedEpisode[] = []

    // 1) metadata layer
    for (const m of detail?.episodes ?? []) {
      const e: MergedEpisode = {
        key: `m${m.number}`,
        number: m.number,
        title: m.title,
        thumbnail: m.thumbnail,
        airDate: m.air_date,
        overview: m.overview,
        pipeline: null,
        source: null,
      }
      byNumber.set(m.number, e)
    }

    // 2) pipeline layer (tracked) — pipeline truth wins for status display.
    for (const p of pipelineEpisodes) {
      if (p.episode_no == null) {
        specials.push({
          key: `p${p.id}`,
          number: null,
          title: p.title,
          thumbnail: null,
          airDate: p.published_at ?? null,
          overview: null,
          pipeline: p,
          source: null,
        })
        continue
      }
      const existing = byNumber.get(p.episode_no)
      if (existing) {
        existing.pipeline = p
        if (!existing.title && p.title) existing.title = p.title
      } else {
        byNumber.set(p.episode_no, {
          key: `p${p.id}`,
          number: p.episode_no,
          title: p.title,
          thumbnail: null,
          airDate: p.published_at ?? null,
          overview: null,
          pipeline: p,
          source: null,
        })
      }
    }

    // 3) source layer (tracked, on-demand) — light up matching cards.
    for (const a of available) {
      const existing = byNumber.get(a.number)
      if (existing) {
        existing.source = a
      } else {
        byNumber.set(a.number, {
          key: `s${a.number}`,
          number: a.number,
          title: a.title || null,
          thumbnail: null,
          airDate: null,
          overview: null,
          pipeline: null,
          source: a,
        })
      }
    }

    const numbered = [...byNumber.values()].sort((x, y) => (x.number ?? 0) - (y.number ?? 0))
    return [...numbered, ...specials]
  })

  // ---- Relations / recommendations → DiscoveryItem shape for DiscoveryCard ----
  function toDiscoveryItem(r: AnilistRelatedEntry): DiscoveryItem {
    return {
      anilist_id: r.anilist_id,
      romaji_title: r.title_romaji ?? '',
      english_title: r.title_english ?? '',
      format: r.format ?? '',
      status: r.status ?? '',
      episode_count: null,
      cover_image: r.cover_image,
      banner_image: r.cover_image,
      clear_logo_url: '',
      cover_color: r.cover_color ?? '',
      season: '',
      season_year: null,
      is_adult: false,
    }
  }
</script>

<div class="flex h-full flex-col overflow-y-auto">
  {#if loading}
    <div class="flex flex-1 items-center justify-center text-[var(--color-muted)]">
      <Spinner size={30} />
    </div>
  {:else if error && !series && !preview && !detail}
    <div class="flex flex-1 flex-col items-center justify-center gap-4 text-center px-6">
      <p class="text-sm text-[var(--color-error)]">{error}</p>
      <div class="flex items-center gap-2">
        {#if numAnilist != null}
          <Button onclick={() => loadDetail(numAnilist)}>Retry</Button>
        {/if}
        <Button variant="outline" onclick={() => navigate('/')}>Back to home</Button>
      </div>
    </div>
  {:else if series || preview || detail}
    <!-- ─── Cinematic header ─────────────────────────────────────── -->
    <section
      class="relative w-full shrink-0 overflow-hidden"
      style="--accent: {accent}; --accent-rgb: {accentRgb}; --accent-fg: {accentFg}; --accent-text: {accentTxt}; --accent-text-rgb: {accentTxtRgb};"
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
        <Button variant="ghost" size="sm" onclick={() => navigate(series ? '/library' : '/')}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M19 12H5M12 5l-7 7 7 7" stroke-linecap="round" stroke-linejoin="round"/></svg>
          {series ? 'Library' : 'Home'}
        </Button>
      </div>

      <!-- Content -->
      <div class="relative px-6 sm:px-10 pt-10 pb-9 sm:pt-16 sm:pb-12">
        <div class="flex flex-col gap-6 sm:flex-row sm:items-end sm:gap-7 animate-fade-up">
          <!-- Poster -->
          <div class="shrink-0">
            <div class="w-32 sm:w-44 overflow-hidden ring-1 ring-white/10 shadow-[0_24px_60px_-20px_rgba(0,0,0,0.85)]">
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
                <span class="inline-flex items-center gap-1.5 bg-white/5 px-3 py-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-[var(--color-text-dim)] ring-1 ring-white/10">
                  <span class="h-1.5 w-1.5 rounded-full bg-[var(--accent-text)]"></span>
                  Season {series.season_number}
                </span>
                {#if series.feed_title}
                  <span class="text-[11px] text-[var(--color-muted)]">via {series.feed_title}</span>
                {/if}
              {:else}
                <span class="inline-flex items-center gap-1.5 bg-white/5 px-3 py-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-[var(--color-text-dim)] ring-1 ring-white/10">
                  <span class="h-1.5 w-1.5 rounded-full bg-[var(--accent-text)]"></span>
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
              {#if series && bucket}
                <span class={`inline-flex items-center border px-2.5 py-1 text-[11px] font-medium backdrop-blur-sm ${watchStatusColor(bucket)}`}>
                  {watchStatusLabel(bucket)}
                </span>
              {/if}
              {#if format}
                <span class="inline-flex items-center bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)] ring-1 ring-white/10">{format}</span>
              {/if}
              {#if series && fmtAiring(series.airing_status)}
                <span class="inline-flex items-center bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)] ring-1 ring-white/10">{fmtAiring(series.airing_status)}</span>
              {:else if preview?.status || detail?.airing_status}
                <span class="inline-flex items-center bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)] ring-1 ring-white/10">{titleCase(preview?.status || detail?.airing_status || '')}</span>
              {/if}
              {#if (series?.episode_count ?? preview?.episode_count ?? detail?.episode_count)}
                <span class="inline-flex items-center bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)] ring-1 ring-white/10">{series?.episode_count ?? preview?.episode_count ?? detail?.episode_count} episodes</span>
              {/if}
              {#if series}
                <span class="inline-flex items-center gap-1.5 bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium tabular-nums text-[var(--color-text-dim)] ring-1 ring-white/10">
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 9h18" stroke-linecap="round"/></svg>
                  {archivedCount}/{totalCount} archived
                </span>
              {:else if preview && preview.season_year}
                <span class="inline-flex items-center bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)] ring-1 ring-white/10">{titleCase(preview.season)} {preview.season_year}</span>
              {/if}
            </div>

            <!-- actions -->
            <div class="mt-6 flex flex-wrap items-center gap-2">
              {#if preview && !series}
                <!-- Untracked: the single tracked-series creation path -->
                <Button onclick={() => track(numAnilist!)} disabled={tracking}>
                  {#if tracking}
                    <Spinner size={14}/>
                    Subscribing…
                  {:else}
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9M13.7 21a2 2 0 0 1-3.4 0" stroke-linecap="round" stroke-linejoin="round"/></svg>
                    Subscribe
                  {/if}
                </Button>
              {:else if !series && numAnilist != null}
                <!-- Untracked, no preview hint yet (detail-only paint) -->
                <Button onclick={() => track(numAnilist!)} disabled={tracking}>
                  {#if tracking}
                    <Spinner size={14}/>
                    Subscribing…
                  {:else}
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9M13.7 21a2 2 0 0 1-3.4 0" stroke-linecap="round" stroke-linejoin="round"/></svg>
                    Subscribe
                  {/if}
                </Button>
              {:else if series}
                <!-- Tracked: watch-status switch -->
                <div
                  class="flex shrink-0 overflow-hidden border border-[var(--color-border)] bg-[var(--color-surface-2)]"
                  role="group"
                  aria-label="Watch status"
                >
                  {#each [{ value: 'watching', label: 'Watching' }, { value: 'on_hold', label: 'On Hold' }, { value: 'dropped', label: 'Dropped' }] as opt (opt.value)}
                    {@const on = series.status === opt.value}
                    <button
                      type="button"
                      class="px-3 py-1.5 text-[13px] font-medium transition-colors
                        {on
                          ? 'bg-[rgb(var(--accent-rgb)/0.2)] text-[var(--color-text)]'
                          : 'text-[var(--color-muted)] hover:bg-white/5 hover:text-[var(--color-text)]'}
                        disabled:opacity-50 disabled:cursor-not-allowed"
                      aria-pressed={on}
                      disabled={statusBusy}
                      onclick={() => setStatus(opt.value as 'watching' | 'on_hold' | 'dropped')}
                    >{opt.label}</button>
                  {/each}
                </div>

                <Button variant="ghost" onclick={() => (unsubscribeOpen = true)} title="Unsubscribe (removes tracking; keeps files)">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9M13.7 21a2 2 0 0 1-3.4 0M1 1l22 22" stroke-linecap="round" stroke-linejoin="round"/></svg>
                  Unsubscribe
                </Button>

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

              {#if detail?.trailer}
                <Button variant="outline" onclick={openTrailer} title="Open trailer on YouTube">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" stroke="none"><path d="M8 5v14l11-7z"/></svg>
                  Trailer
                </Button>
              {/if}
            </div>

            {#if series && (series.status === 'on_hold' || series.status === 'dropped')}
              <p class="mt-3 text-xs text-[var(--color-muted)] max-w-xl">
                {series.status === 'on_hold' ? 'On Hold' : 'Dropped'} — background auto-download is off. Files are kept. Set it back to Watching to resume polling.
              </p>
            {/if}
          </div>
        </div>
      </div>
    </section>

    <!-- ─── Synopsis ─────────────────────────────────────────────── -->
    <section class="px-6 sm:px-10 pt-6">
      {#if detail}
        {#if detail.description}
          <p class="max-w-3xl text-sm leading-relaxed text-[var(--color-text-dim)] {synopsisExpanded ? '' : 'line-clamp-4'}">
            {detail.description}
          </p>
          <button
            type="button"
            class="mt-1.5 text-xs font-semibold text-[var(--accent-text)] hover:brightness-125 transition"
            onclick={() => (synopsisExpanded = !synopsisExpanded)}
          >
            {synopsisExpanded ? 'Show less' : 'Show more'}
          </button>
        {/if}
      {:else if detailLoading}
        <div class="max-w-3xl space-y-2">
          <div class="h-3 w-full bg-white/[0.05] animate-pulse"></div>
          <div class="h-3 w-[92%] bg-white/[0.05] animate-pulse"></div>
          <div class="h-3 w-[96%] bg-white/[0.05] animate-pulse"></div>
          <div class="h-3 w-[60%] bg-white/[0.05] animate-pulse"></div>
        </div>
      {/if}
    </section>

    <!-- ─── Genre chips ──────────────────────────────────────────── -->
    {#if detail?.genres?.length}
      <section class="px-6 sm:px-10 pt-5">
        <div class="flex flex-wrap gap-2">
          {#each detail.genres as g (g)}
            <span class="inline-flex items-center bg-white/[0.05] px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)] ring-1 ring-white/[0.08]">{g}</span>
          {/each}
        </div>
      </section>
    {/if}

    <!-- ─── About strip ──────────────────────────────────────────── -->
    {#if aboutPairs.length}
      <section class="px-6 sm:px-10 pt-6">
        <div class="flex flex-wrap gap-x-8 gap-y-3">
          {#each aboutPairs as pair (pair.label)}
            <div class="min-w-0">
              <div class="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">{pair.label}</div>
              <div class="mt-0.5 text-sm font-medium text-[var(--color-text)] tabular-nums">{pair.value}</div>
            </div>
          {/each}
        </div>
      </section>
    {:else if detailLoading}
      <section class="px-6 sm:px-10 pt-6">
        <div class="flex flex-wrap gap-x-8 gap-y-3">
          {#each Array(5) as _, i (i)}
            <div class="space-y-1.5">
              <div class="h-2.5 w-14 bg-white/[0.05] animate-pulse"></div>
              <div class="h-3.5 w-20 bg-white/[0.05] animate-pulse"></div>
            </div>
          {/each}
        </div>
      </section>
    {/if}

    <!-- ─── Unified episode grid ─────────────────────────────────── -->
    <section class="px-6 sm:px-10 pt-8">
      <!-- section header -->
      <div class="mb-4 flex items-center gap-3">
        <h2 class="text-[15px] font-semibold tracking-tight text-[var(--color-text)]">Episodes</h2>
        {#if mergedEpisodes.length}
          <span class="text-xs font-medium text-[var(--color-muted)] tabular-nums">{mergedEpisodes.length}</span>
        {/if}

        {#if series}
          <div class="ml-auto flex items-center gap-2">
            {#if pipelineEpisodes.length}
              <Button variant="ghost" size="sm" onclick={toggleAll}>
                {allSelected ? 'Clear' : 'Select all'}
              </Button>
            {/if}
            <Button variant="secondary" size="sm" onclick={loadAvailable} disabled={availableLoading}>
              {#if availableLoading}<Spinner size={13}/>{:else}
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 21l-6-6m2-5a7 7 0 1 1-14 0 7 7 0 0 1 14 0z" stroke-linecap="round" stroke-linejoin="round"/></svg>
              {/if}
              {availableChecked ? 'Re-check source' : 'Check source'}
            </Button>
          </div>
        {/if}
      </div>

      {#if availableWarnings.length}
        <div
          class="mb-4 flex gap-3 border border-[var(--color-warning)]/30 bg-[var(--color-warning)]/10 px-4 py-3"
          role="alert"
        >
          <svg class="mt-0.5 shrink-0 text-[var(--color-warning)]" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" stroke-linecap="round" stroke-linejoin="round"/>
            <path d="M12 9v4M12 17h.01" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
          <div class="min-w-0 flex-1 space-y-1.5">
            <p class="text-sm font-medium text-[var(--color-warning)]">
              Some sources failed — you may not see all (or any) episodes.
            </p>
            <ul class="space-y-0.5">
              {#each availableWarnings as w (w)}
                <li class="break-all font-mono text-[11px] leading-relaxed text-[var(--color-text-dim)]/70">{w}</li>
              {/each}
            </ul>
          </div>
        </div>
      {/if}

      {#if mergedEpisodes.length === 0}
        {#if detailLoading}
          <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
            {#each Array(6) as _, i (i)}
              <div class="flex gap-3 border border-[var(--color-border)] bg-[var(--color-surface)] p-2.5">
                <div class="aspect-video w-40 shrink-0 bg-white/[0.05] animate-pulse"></div>
                <div class="min-w-0 flex-1 space-y-2 py-1">
                  <div class="h-3 w-[70%] bg-white/[0.05] animate-pulse"></div>
                  <div class="h-2.5 w-full bg-white/[0.04] animate-pulse"></div>
                  <div class="h-2.5 w-[80%] bg-white/[0.04] animate-pulse"></div>
                </div>
              </div>
            {/each}
          </div>
        {:else}
          <div class="flex flex-col items-center justify-center gap-4 border border-[var(--color-border)] bg-[var(--color-surface)] px-6 py-16 text-center">
            <div class="flex h-14 w-14 items-center justify-center bg-white/[0.04] text-[var(--color-faint)] ring-1 ring-white/10">
              <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.25"><path d="M9 12h6m-6 4h6m2 5H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5.586a1 1 0 0 1 .707.293l5.414 5.414a1 1 0 0 1 .293.707V19a2 2 0 0 1-2 2z" stroke-linecap="round" stroke-linejoin="round"/></svg>
            </div>
            <div class="space-y-1.5">
              <h3 class="text-base font-semibold tracking-tight">
                {availableWarnings.length ? 'Source check failed' : 'No episodes yet'}
              </h3>
              <p class="max-w-sm text-sm text-[var(--color-muted)]">
                {#if availableWarnings.length}The source(s) errored, so no episodes could be listed — this is a source problem, not an empty catalogue. See the warning above.{:else if series}The auto-downloader will grab new episodes as they air. Or run a source check above.{:else}Episode data will appear once it's available.{/if}
              </p>
            </div>
          </div>
        {/if}
      {:else}
        <ul class="grid grid-cols-1 gap-3 lg:grid-cols-2">
          {#each mergedEpisodes as ep (ep.key)}
            {@const p = ep.pipeline}
            {@const progress = p ? getEpisodeProgress(p) : null}
            {@const epStatus = p ? liveStatus(p) : null}
            {@const future = isFuture(ep.airDate)}
            {@const isSelected = p ? selected.has(p.id) : false}
            {@const overall = p ? episodeOverall(p) : 0}
            {@const stage = p ? episodeStage(p) : 'queued'}
            {@const done = stage === 'done'}
            {@const greenMix = fillGreenMix(overall, done)}
            <li
              class="group relative flex gap-3 border bg-[var(--color-surface)] p-2.5 transition-colors duration-200
                {isSelected ? 'border-[var(--accent-text)] bg-[rgb(var(--accent-rgb)/0.06)]' : stage === 'error' ? 'border-[var(--color-error)]/40' : 'border-[var(--color-border)] hover:border-[var(--color-border-strong)]'}
                {future ? 'opacity-55' : ''}"
              style={p ? `--fill-color: color-mix(in oklab, var(--accent-text) ${(1 - greenMix) * 100}%, var(--color-success) ${greenMix * 100}%);` : ''}
            >
              <!-- background fill-sweep (pipeline-backed, in-progress episodes) -->
              {#if p && overall > 0}
                <div
                  class="pointer-events-none absolute inset-0 z-0 transition-[width] duration-700 ease-[cubic-bezier(0.32,0.72,0,1)]"
                  style="width: {overall}%; background: linear-gradient(90deg, rgb(from var(--fill-color) r g b / 0.20), rgb(from var(--fill-color) r g b / 0.07) 70%, rgb(from var(--fill-color) r g b / 0.14));"
                ></div>
                {#if overall < 100}
                  <div
                    class="pointer-events-none absolute inset-y-0 z-0 w-px transition-[left] duration-700 ease-[cubic-bezier(0.32,0.72,0,1)]"
                    style="left: {overall}%; background: rgb(from var(--fill-color) r g b / 0.5);"
                  ></div>
                {/if}
              {/if}

              <!-- ✓ completion badge -->
              {#if done}
                <span class="absolute right-2 top-2 z-[6] inline-flex h-5 w-5 items-center justify-center bg-[var(--color-success)] text-black" aria-hidden="true">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><path d="M20 6 9 17l-5-5" stroke-linecap="round" stroke-linejoin="round"/></svg>
                </span>
              {/if}

              <!-- thumbnail -->
              <div class="relative z-[5] aspect-video w-40 shrink-0 overflow-hidden bg-[var(--color-surface-2)] pointer-events-none">
                <!-- tinted placeholder (always rendered; the image layers on top when present) -->
                <div
                  class="absolute inset-0 z-0 flex items-center justify-center"
                  style="background: radial-gradient(120% 120% at 50% 0%, rgb(var(--accent-rgb) / 0.28), var(--color-surface-2));"
                >
                  <span class="text-lg font-bold tabular-nums text-white/70">
                    {ep.number != null ? `E${String(ep.number).padStart(2, '0')}` : 'SP'}
                  </span>
                </div>

                {#if ep.thumbnail}
                  <img
                    src={ep.thumbnail}
                    alt=""
                    loading="lazy"
                    class="relative z-[1] h-full w-full object-cover"
                    onerror={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none' }}
                  />
                {/if}

                <!-- selection checkbox (tracked, pipeline-backed only) -->
                {#if series && p}
                  <label class="absolute left-1.5 top-1.5 z-10 flex cursor-pointer items-center pointer-events-auto">
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onclick={(e) => e.stopPropagation()}
                      onchange={() => toggleEpisode(p.id)}
                      class="h-4 w-4 cursor-pointer border-[var(--color-border-strong)] bg-black/50 accent-[var(--accent-text)]"
                      aria-label={`Select episode ${ep.number ?? ep.title ?? ''}`}
                    />
                  </label>
                {/if}
              </div>

              <!-- body -->
              <div class="relative z-[5] flex min-w-0 flex-1 flex-col py-0.5 pointer-events-none">
                <div class="flex items-baseline gap-2">
                  <span class="shrink-0 text-[11px] font-bold tabular-nums text-[var(--color-text-dim)]">
                    {ep.number != null ? `E${String(ep.number).padStart(2, '0')}` : 'SP'}
                  </span>
                  <span class="truncate text-sm font-medium text-[var(--color-text)]">
                    {ep.title || (ep.number != null ? `Episode ${ep.number}` : 'Special')}
                  </span>
                </div>

                {#if ep.overview}
                  <p class="mt-1 line-clamp-2 text-xs leading-relaxed text-[var(--color-muted)]">{ep.overview}</p>
                {/if}

                <!-- live stage + percent (the full-card fill-sweep shows overall progress) -->
                {#if p && (stage === 'downloading' || stage === 'encoding')}
                  {@const tick = progress && 'percent' in progress ? progress : null}
                  <div class="mt-1.5 flex items-center gap-2 text-[11px] tabular-nums">
                    <span class="font-semibold text-[var(--color-text-dim)]">{Math.round(overall)}%</span>
                    {#if tick && 'speed_bps' in tick}
                      <span class="text-[var(--color-muted)]">{formatBytes(tick.speed_bps)}/s</span>
                      {#if tick.peers}<span class="text-[var(--color-faint)]">{tick.peers} peers</span>{/if}
                    {:else if tick && 'speed' in tick && tick.speed}
                      <span class="text-[var(--color-muted)]">{tick.resolution}p · {tick.speed}</span>
                    {/if}
                  </div>
                {/if}

                {#if epStatus === 'error' && p?.error_message}
                  <p class="mt-1 truncate text-xs text-[var(--color-error)]" title={p.error_message}>{p.error_message}</p>
                {/if}

                <!-- footer row: air date / status / outputs / actions -->
                <div class="mt-auto flex items-center gap-2 pt-2">
                  {#if future}
                    <span class="text-[11px] font-medium text-[var(--color-muted)]">airs {typeof ep.airDate === 'number' ? formatDate(ep.airDate) : ep.airDate}</span>
                  {:else if ep.airDate}
                    <span class="text-[11px] text-[var(--color-faint)]">{relativeTime(ep.airDate)}</span>
                  {/if}

                  {#if p && epStatus}
                    <span class="text-[11px] font-medium {statusColor(epStatus)}">{epStatus}</span>
                  {/if}

                  <!-- output resolution chips -->
                  {#if p?.outputs?.length}
                    <div class="flex flex-wrap gap-1">
                      {#each p.outputs as out (out.id)}
                        <span
                          class="bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] font-medium tabular-nums ring-1 ring-[var(--color-border)] {statusColor(out.status)}"
                          title={out.error_message ?? out.status}
                        >{out.resolution}p</span>
                      {/each}
                    </div>
                  {/if}

                  <div class="ml-auto flex items-center gap-1.5 pointer-events-auto">
                    {#if series && ep.source}
                      <Button
                        size="sm"
                        onclick={() => downloadAvailable(ep.source!)}
                        disabled={downloadingEp === ep.source.source_url}
                        title={`${ep.source.resolution}${ep.source.size ? ' · ' + formatBytes(ep.source.size) : ''}`}
                      >
                        {#if downloadingEp === ep.source.source_url}<Spinner size={12}/>{:else}
                          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25"><path d="M12 3v12m0 0 4-4m-4 4-4-4M5 21h14" stroke-linecap="round" stroke-linejoin="round"/></svg>
                        {/if}
                        Download
                      </Button>
                    {/if}
                    {#if p && epStatus === 'error'}
                      <Button
                        variant="ghost"
                        size="icon"
                        class="h-7 w-7"
                        onclick={() => api.retryEpisode(p.id).then(() => loadTracked())}
                        title="Retry episode"
                      >
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25"><path d="M1 4v6h6M23 20v-6h-6" stroke-linecap="round" stroke-linejoin="round"/><path d="M20.49 9A9 9 0 0 0 5.64 5.64L1 10m22 4l-4.64 4.36A9 9 0 0 1 3.51 15" stroke-linecap="round" stroke-linejoin="round"/></svg>
                      </Button>
                    {/if}
                  </div>
                </div>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    <!-- ─── Relations ────────────────────────────────────────────── -->
    {#if detail?.relations?.length}
      <section class="px-6 sm:px-10 pt-10">
        <Carousel title="Relations" count={detail.relations.length}>
          {#each detail.relations as r (r.anilist_id + ':' + (r.relation_type ?? ''))}
            <div class="w-[150px] shrink-0 snap-start">
              {#if r.relation_type}
                <div class="mb-1.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">{titleCase(r.relation_type)}</div>
              {/if}
              <DiscoveryCard item={toDiscoveryItem(r)} />
            </div>
          {/each}
        </Carousel>
      </section>
    {/if}

    <!-- ─── Recommendations ──────────────────────────────────────── -->
    {#if detail?.recommendations?.length}
      <section class="px-6 sm:px-10 pt-10 pb-16">
        <Carousel title="Recommendations" count={detail.recommendations.length}>
          {#each detail.recommendations as r (r.anilist_id)}
            <div class="w-[150px] shrink-0 snap-start">
              <DiscoveryCard item={toDiscoveryItem(r)} />
            </div>
          {/each}
        </Carousel>
      </section>
    {:else}
      <div class="pb-16"></div>
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
        class="h-9 w-full cursor-pointer border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] transition-colors focus:border-[var(--accent-text)] focus:outline-none"
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
            class="border px-3.5 py-1.5 text-sm font-medium transition-colors duration-200
              {on
                ? 'border-[var(--accent-text)] bg-[rgb(var(--accent-rgb)/0.15)] text-[var(--color-text)]'
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

<!-- ─── Unsubscribe confirmation ───────────────────────────────── -->
<Modal bind:open={unsubscribeOpen} title="Unsubscribe from {title}?">
  {#snippet footer()}
    <Button variant="ghost" onclick={() => { unsubscribeOpen = false }}>Cancel</Button>
    <Button variant="destructive" onclick={unsubscribe} disabled={unsubscribing}>
      {#if unsubscribing}<Spinner size={14}/>{/if}
      Unsubscribe
    </Button>
  {/snippet}

  <div class="space-y-3 text-sm text-[var(--color-text-dim)]">
    <p>This removes the series from your library and stops all tracking. Its episodes, feeds, and encode records are deleted.</p>
    <p class="text-[var(--color-muted)]">Downloaded and encoded files on disk are <span class="font-medium text-[var(--color-text)]">kept</span> — they remain in your library folder as orphaned files.</p>
  </div>
</Modal>
