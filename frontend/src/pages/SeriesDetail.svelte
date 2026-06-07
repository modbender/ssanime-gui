<script lang="ts">
  import { navigate } from 'svelte-routing'
  import { api, type SeriesDetail as SeriesDetailType, type EpisodeDetail, type Profile } from '$lib/api'
  import Badge from '$lib/components/Badge.svelte'
  import Button from '$lib/components/Button.svelte'
  import Modal from '$lib/components/Modal.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import ProgressBar from '$lib/components/ProgressBar.svelte'
  import { statusColor, derivedStatusColor, derivedStatusLabel, formatBytes } from '$lib/utils'
  import { sseState } from '$lib/sse.svelte'

  let { id }: { id: string } = $props()
  const numId = $derived(Number(id))

  let series = $state<SeriesDetailType | null>(null)
  let profiles = $state<Profile[]>([])
  let loading = $state(true)
  let error = $state('')

  // Selections
  let selected = $state<Set<number>>(new Set())

  // Encode modal
  let encodeOpen = $state(false)
  let encodeProfileId = $state<number | null>(null)
  let encodeResolutions = $state<number[]>([1080, 720])
  let encoding = $state(false)

  // Scan state
  let scanning = $state(false)

  async function load() {
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

  $effect(() => { if (numId) load() })

  const allSelected = $derived(
    series?.episodes.length ? selected.size === series.episodes.length : false
  )

  function toggleAll() {
    if (allSelected) {
      selected = new Set()
    } else {
      selected = new Set(series!.episodes.map(e => e.id))
    }
  }

  function toggleEpisode(id: number) {
    const s = new Set(selected)
    if (s.has(id)) s.delete(id); else s.add(id)
    selected = s
  }

  async function toggleSubscribe() {
    if (!series) return
    try {
      series = await api.patchSeries(numId, { subscribed: !series.subscribed })
    } catch {}
  }

  async function toggleFavorite() {
    if (!series) return
    try {
      series = await api.patchSeries(numId, { favorite: !series.favorite })
    } catch {}
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
      await load()
    } catch (e: any) { alert(e.message) }
    finally { encoding = false }
  }

  async function scan() {
    scanning = true
    try {
      const eps = await api.scanEpisodes(numId)
      if (series) series = { ...series, episodes: eps }
    } catch (e: any) { alert(e.message) }
    finally { scanning = false }
  }

  const resolutionOptions = [2160, 1080, 720, 480, 360]

  function toggleResolution(res: number) {
    if (encodeResolutions.includes(res)) {
      encodeResolutions = encodeResolutions.filter(r => r !== res)
    } else {
      encodeResolutions = [...encodeResolutions, res]
    }
  }

  function getEpisodeProgress(ep: EpisodeDetail) {
    return sseState.downloadProgress[ep.id] ?? sseState.encodeProgress[ep.id] ?? null
  }

  function liveStatus(ep: EpisodeDetail) {
    return sseState.episodeStatus[ep.id] ?? ep.status
  }
</script>

<div class="flex flex-col h-full">
  <!-- Back -->
  <div class="flex items-center gap-2 px-6 pt-4 pb-2">
    <Button variant="ghost" size="sm" onclick={() => navigate('/')}>
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M19 12H5M12 5l-7 7 7 7" stroke-linecap="round" stroke-linejoin="round"/></svg>
      Library
    </Button>
  </div>

  {#if loading}
    <div class="flex-1 flex items-center justify-center text-[#6b6b80]">
      <Spinner size={28}/>
    </div>
  {:else if error}
    <div class="flex-1 flex items-center justify-center text-red-400 text-sm">{error}</div>
  {:else if series}
    <!-- Series header -->
    <div class="relative px-6 pb-5 border-b border-[#2a2a35]">
      {#if series.banner_image_url}
        <div class="absolute inset-0 overflow-hidden rounded-t-none">
          <img src={series.banner_image_url} alt="" class="w-full h-full object-cover opacity-10" />
          <div class="absolute inset-0 bg-gradient-to-b from-transparent to-[#0a0a0f]"></div>
        </div>
      {/if}
      <div class="relative flex gap-5">
        <!-- Poster -->
        <div class="shrink-0 w-28 rounded-xl overflow-hidden shadow-xl border border-[#2a2a35]">
          {#if series.cover_image_url}
            <img src={series.cover_image_url} alt={series.title} class="w-full aspect-[2/3] object-cover" />
          {:else}
            <div class="w-full aspect-[2/3] bg-[#18181f]"></div>
          {/if}
        </div>

        <!-- Meta -->
        <div class="flex-1 min-w-0 pt-1">
          <div class="flex items-start gap-3 flex-wrap mb-2">
            <h1 class="text-xl font-semibold text-[#e8e8f0] leading-tight">{series.english_title || series.romaji_title || series.title}</h1>
            <Badge class={derivedStatusColor(series.derived_status)}>
              {derivedStatusLabel(series.derived_status)}
            </Badge>
          </div>
          {#if series.romaji_title && series.english_title}
            <p class="text-[#6b6b80] text-sm mb-1">{series.romaji_title}</p>
          {/if}
          <p class="text-[#6b6b80] text-xs mb-3">
            {series.format ?? ''}{series.format && series.airing_status ? ' · ' : ''}{series.airing_status ?? ''}
            {#if series.episode_count} · {series.episode_count} episodes{/if}
            · Season {series.season_number}
          </p>

          <div class="flex items-center gap-2 flex-wrap">
            <!-- Subscribe -->
            <Button
              variant={series.subscribed ? 'default' : 'outline'}
              size="sm"
              onclick={toggleSubscribe}
            >
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M15 17h5l-1.405-1.405A2.032 2.032 0 0 1 18 14.158V11a6.002 6.002 0 0 0-4-5.659V5a2 2 0 1 0-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 1 1-6 0v-1m6 0H9" stroke-linecap="round" stroke-linejoin="round"/></svg>
              {series.subscribed ? 'Subscribed' : 'Subscribe'}
            </Button>

            <!-- Favorite -->
            <Button
              variant={series.favorite ? 'default' : 'outline'}
              size="sm"
              onclick={toggleFavorite}
            >
              <svg width="12" height="12" viewBox="0 0 24 24" fill={series.favorite ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2.5"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z" stroke-linecap="round" stroke-linejoin="round"/></svg>
              {series.favorite ? 'Favorited' : 'Favorite'}
            </Button>

            <!-- Scan -->
            <Button variant="secondary" size="sm" onclick={scan} disabled={scanning}>
              {#if scanning}<Spinner size={12}/>{:else}
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M21 21l-6-6m2-5a7 7 0 1 1-14 0 7 7 0 0 1 14 0z" stroke-linecap="round" stroke-linejoin="round"/></svg>
              {/if}
              Scan torrents
            </Button>

            <!-- Bulk encode -->
            {#if selected.size > 0}
              <Button onclick={() => { encodeOpen = true }}>
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M5 3l14 9-14 9V3z" stroke-linecap="round" stroke-linejoin="round"/></svg>
                Download &amp; Encode ({selected.size})
              </Button>
            {/if}
          </div>
        </div>
      </div>
    </div>

    <!-- Episodes list -->
    <div class="flex-1 overflow-y-auto">
      <!-- List header -->
      <div class="flex items-center gap-3 px-6 py-3 border-b border-[#2a2a35] sticky top-0 bg-[#0a0a0f] z-10">
        <input
          type="checkbox"
          checked={allSelected}
          onchange={toggleAll}
          class="rounded border-[#2a2a35] cursor-pointer accent-[#7c6af0]"
          aria-label="Select all episodes"
        />
        <span class="text-xs text-[#6b6b80] uppercase tracking-wider">Episode</span>
        <span class="ml-auto text-xs text-[#6b6b80] uppercase tracking-wider">Status</span>
        <span class="text-xs text-[#6b6b80] uppercase tracking-wider w-28 text-right">Outputs</span>
      </div>

      {#if series.episodes.length === 0}
        <div class="flex flex-col items-center justify-center py-16 text-[#6b6b80] gap-3 text-sm">
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#2a2a35" stroke-width="1.5"><path d="M9 12h6m-6 4h6m2 5H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5.586a1 1 0 0 1 .707.293l5.414 5.414a1 1 0 0 1 .293.707V19a2 2 0 0 1-2 2z"/></svg>
          No episodes. Click "Scan torrents" to find available episodes.
        </div>
      {:else}
        {#each series.episodes as ep (ep.id)}
          {@const progress = getEpisodeProgress(ep)}
          {@const status = liveStatus(ep)}
          <div class="flex items-center gap-3 px-6 py-3 border-b border-[#2a2a35]/50 hover:bg-[#111118]/50 transition-colors">
            <input
              type="checkbox"
              checked={selected.has(ep.id)}
              onchange={() => toggleEpisode(ep.id)}
              class="rounded border-[#2a2a35] cursor-pointer accent-[#7c6af0]"
              aria-label={`Select episode ${ep.episode_no}`}
            />

            <div class="flex-1 min-w-0">
              <div class="flex items-baseline gap-2">
                <span class="text-[#6b6b80] text-xs w-8 shrink-0">
                  {ep.episode_no != null ? `E${String(ep.episode_no).padStart(2, '0')}` : 'SP'}
                </span>
                <span class="text-[#e8e8f0] text-sm truncate">{ep.title ?? `Episode ${ep.episode_no}`}</span>
                {#if ep.release_group}
                  <span class="text-[#6b6b80] text-xs shrink-0">[{ep.release_group}]</span>
                {/if}
                {#if ep.source_size}
                  <span class="text-[#6b6b80] text-xs shrink-0">{formatBytes(ep.source_size)}</span>
                {/if}
              </div>
              {#if progress && 'percent' in progress}
                <div class="mt-1.5">
                  <ProgressBar value={progress.percent} max={100} />
                </div>
              {/if}
            </div>

            <!-- Status -->
            <span class="text-xs font-medium shrink-0 {statusColor(status)}">{status}</span>

            <!-- Outputs -->
            <div class="w-28 flex gap-1 justify-end flex-wrap">
              {#each ep.outputs as out (out.id)}
                <span class="text-xs px-1.5 py-0.5 rounded bg-[#18181f] border border-[#2a2a35] {statusColor(out.status)}" title={out.status}>
                  {out.resolution}p
                </span>
              {/each}
            </div>

            <!-- Actions -->
            <div class="flex gap-1 shrink-0">
              {#if status === 'error'}
                <Button
                  variant="ghost"
                  size="icon"
                  onclick={() => api.retryEpisode(ep.id).then(() => load())}
                  title="Retry"
                >
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M1 4v6h6M23 20v-6h-6" stroke-linecap="round" stroke-linejoin="round"/><path d="M20.49 9A9 9 0 0 0 5.64 5.64L1 10m22 4l-4.64 4.36A9 9 0 0 1 3.51 15" stroke-linecap="round" stroke-linejoin="round"/></svg>
                </Button>
              {/if}
            </div>
          </div>
        {/each}
      {/if}
    </div>
  {/if}
</div>

<!-- Encode modal -->
<Modal bind:open={encodeOpen} title="Download & Encode {selected.size} episode(s)">
  {#snippet footer()}
    <Button variant="ghost" onclick={() => { encodeOpen = false }}>Cancel</Button>
    <Button onclick={startEncode} disabled={encoding || encodeResolutions.length === 0}>
      {#if encoding}<Spinner size={14}/>{/if}
      Start
    </Button>
  {/snippet}

  <div class="space-y-5">
    <!-- Profile picker -->
    <div>
      <label class="block text-xs text-[#6b6b80] mb-2 uppercase tracking-wide">Encode profile</label>
      <select
        bind:value={encodeProfileId}
        class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] focus:outline-none focus:border-[#7c6af0] cursor-pointer"
      >
        <option value={null}>Default profile</option>
        {#each profiles as p (p.id)}
          <option value={p.id}>{p.name}{p.is_builtin ? ' (builtin)' : ''}</option>
        {/each}
      </select>
    </div>

    <!-- Resolution picker -->
    <div>
      <label class="block text-xs text-[#6b6b80] mb-2 uppercase tracking-wide">Output resolutions</label>
      <div class="flex gap-2 flex-wrap">
        {#each resolutionOptions as res}
          <button
            class="px-3 py-1.5 rounded-lg text-sm border transition-colors {encodeResolutions.includes(res) ? 'border-[#7c6af0] bg-[#7c6af0]/15 text-[#e8e8f0]' : 'border-[#2a2a35] text-[#6b6b80] hover:border-[#7c6af0]/50'}"
            onclick={() => toggleResolution(res)}
          >{res}p</button>
        {/each}
      </div>
    </div>
  </div>
</Modal>
