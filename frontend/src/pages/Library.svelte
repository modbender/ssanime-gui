<script lang="ts">
  import { api, type SeriesProgress, type AnilistSearchResult, type EpisodeDetail } from '$lib/api'
  import Button from '$lib/components/Button.svelte'
  import Input from '$lib/components/Input.svelte'
  import Modal from '$lib/components/Modal.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import Hero from '$lib/components/Hero.svelte'
  import Carousel from '$lib/components/Carousel.svelte'
  import PosterCard from '$lib/components/PosterCard.svelte'

  let series = $state<SeriesProgress[]>([])
  let loading = $state(true)
  let error = $state('')

  // Downloading-now row (best-effort; row hidden if the endpoint/data is empty)
  let downloadingIds = $state<Set<number>>(new Set())

  // Filters
  let searchQ = $state('')
  let filterStatus = $state('')
  let filterSubscribed = $state(false)
  let filterFavorite = $state(false)

  // Add modal
  let addOpen = $state(false)
  let anilistQ = $state('')
  let anilistResults = $state<AnilistSearchResult[]>([])
  let anilistLoading = $state(false)
  let adding = $state(false)

  async function load() {
    loading = true
    error = ''
    try {
      series = await api.listSeries()
    } catch (e: any) {
      error = e.message
    } finally {
      loading = false
    }
    loadDownloading()
  }

  async function loadDownloading() {
    try {
      const snap = await api.getQueue()
      const active = [...(snap.downloading ?? []), ...(snap.encoding ?? [])]
      downloadingIds = new Set(active.map((ep: EpisodeDetail) => ep.series_id))
    } catch {
      downloadingIds = new Set()
    }
  }

  $effect(() => { load() })

  // ---- Derived collections ----

  const filtered = $derived(series.filter(s => {
    const q = searchQ.toLowerCase()
    if (searchQ &&
        !s.title.toLowerCase().includes(q) &&
        !(s.english_title ?? '').toLowerCase().includes(q) &&
        !(s.romaji_title ?? '').toLowerCase().includes(q)) return false
    if (filterStatus && s.derived_status !== filterStatus) return false
    if (filterSubscribed && !s.subscribed) return false
    if (filterFavorite && !s.favorite) return false
    return true
  }))

  // Hero: prefer subscribed + airing, newest first; else any airing; else newest subscribed; else newest overall.
  const heroItems = $derived.by(() => {
    const byModified = (a: SeriesProgress, b: SeriesProgress) => b.modified_at - a.modified_at
    const subAiring = series.filter(s => s.subscribed && s.derived_status === 'airing').sort(byModified)
    if (subAiring.length) return subAiring.slice(0, 6)
    const airing = series.filter(s => s.derived_status === 'airing').sort(byModified)
    if (airing.length) return airing.slice(0, 6)
    const subbed = series.filter(s => s.subscribed).sort(byModified)
    if (subbed.length) return subbed.slice(0, 6)
    return [...series].sort(byModified).slice(0, 1)
  })

  const airingSubscribed = $derived(
    series
      .filter(s => s.subscribed || s.derived_status === 'airing')
      .sort((a, b) => b.modified_at - a.modified_at),
  )

  const recentlyArchived = $derived(
    series
      .filter(s => s.episode_archived > 0)
      .sort((a, b) => b.modified_at - a.modified_at),
  )

  const downloadingNow = $derived(
    series.filter(s => downloadingIds.has(s.id)),
  )

  // ---- Actions ----

  async function searchAnilist() {
    if (!anilistQ.trim()) return
    anilistLoading = true
    try {
      anilistResults = await api.searchAnilist(anilistQ)
    } catch {}
    finally { anilistLoading = false }
  }

  async function addSeries(result: AnilistSearchResult) {
    adding = true
    try {
      await api.createSeries({ anilist_id: result.id })
      addOpen = false
      anilistQ = ''
      anilistResults = []
      await load()
    } catch (e: any) {
      alert(e.message)
    } finally {
      adding = false
    }
  }

  async function toggleSubscribe(s: SeriesProgress) {
    const next = !s.subscribed
    // optimistic
    series = series.map(x => x.id === s.id ? { ...x, subscribed: next } : x)
    try {
      await api.patchSeries(s.id, { subscribed: next })
    } catch (e: any) {
      // revert on failure
      series = series.map(x => x.id === s.id ? { ...x, subscribed: !next } : x)
      alert(e.message)
    }
  }

  const hasFilters = $derived(!!(searchQ || filterStatus || filterSubscribed || filterFavorite))
</script>

<div class="flex flex-col h-full overflow-y-auto">
  {#if loading}
    <div class="flex flex-1 items-center justify-center text-[var(--color-muted)]">
      <Spinner size={30} />
    </div>
  {:else if error}
    <div class="flex flex-1 items-center justify-center text-[var(--color-error)] text-sm">{error}</div>
  {:else if series.length === 0}
    <!-- Global empty state -->
    <div class="flex flex-1 flex-col items-center justify-center gap-5 px-6 text-center">
      <div class="w-16 h-16 rounded-2xl bg-white/[0.04] ring-1 ring-white/10 flex items-center justify-center text-[var(--color-faint)]">
        <svg width="30" height="30" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.25"><rect x="3" y="3" width="18" height="18" rx="3"/><path d="m3 16 5-5 4 4 3-3 6 6" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </div>
      <div class="space-y-1.5">
        <h2 class="text-lg font-semibold tracking-tight">Your library is empty</h2>
        <p class="text-sm text-[var(--color-muted)] max-w-sm">Add a series from AniList to start auto-downloading, re-encoding, and archiving episodes.</p>
      </div>
      <Button size="lg" onclick={() => { addOpen = true }}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25"><path d="M12 5v14M5 12h14" stroke-linecap="round"/></svg>
        Add your first series
      </Button>
    </div>
  {:else}
    <!-- HERO -->
    <Hero items={heroItems} onToggleSubscribe={toggleSubscribe} />

    <div class="px-6 sm:px-10 pb-16 -mt-2 space-y-12">
      <!-- CAROUSELS -->
      {#if downloadingNow.length > 0}
        <Carousel title="Downloading now" count={downloadingNow.length}>
          {#each downloadingNow as s (s.id)}
            <div class="snap-start shrink-0 w-[150px]"><PosterCard series={s} /></div>
          {/each}
        </Carousel>
      {/if}

      {#if airingSubscribed.length > 0}
        <Carousel title="Airing &amp; subscribed" count={airingSubscribed.length}>
          {#each airingSubscribed as s (s.id)}
            <div class="snap-start shrink-0 w-[150px]"><PosterCard series={s} /></div>
          {/each}
        </Carousel>
      {/if}

      {#if recentlyArchived.length > 0}
        <Carousel title="Recently archived" count={recentlyArchived.length}>
          {#each recentlyArchived as s (s.id)}
            <div class="snap-start shrink-0 w-[150px]"><PosterCard series={s} /></div>
          {/each}
        </Carousel>
      {/if}

      <!-- LIBRARY GRID -->
      <section class="animate-fade-up">
        <div class="flex flex-col gap-3 mb-5 sm:flex-row sm:items-center sm:justify-between">
          <div class="flex items-baseline gap-2.5">
            <h2 class="text-[15px] font-semibold tracking-tight">All series</h2>
            <span class="text-xs font-medium text-[var(--color-muted)] tabular-nums">{filtered.length}</span>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <div class="relative">
              <svg class="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3" stroke-linecap="round"/></svg>
              <Input bind:value={searchQ} placeholder="Search series…" class="w-52 pl-9" />
            </div>
            <select
              bind:value={filterStatus}
              class="h-9 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors cursor-pointer"
            >
              <option value="">All statuses</option>
              <option value="airing">Airing</option>
              <option value="up_to_date">Up to date</option>
              <option value="completed">Completed</option>
              <option value="incomplete">Incomplete</option>
              <option value="not_aired">Not aired</option>
              <option value="cancelled">Cancelled</option>
            </select>
            <Button
              variant={filterSubscribed ? 'default' : 'outline'}
              size="md"
              onclick={() => { filterSubscribed = !filterSubscribed }}
            >Subscribed</Button>
            <Button
              variant={filterFavorite ? 'default' : 'outline'}
              size="md"
              onclick={() => { filterFavorite = !filterFavorite }}
            >Favorites</Button>
            <Button onclick={() => { addOpen = true }}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M12 5v14M5 12h14" stroke-linecap="round"/></svg>
              Add series
            </Button>
          </div>
        </div>

        {#if filtered.length === 0}
          <div class="flex flex-col items-center justify-center gap-3 py-20 text-center">
            <div class="w-12 h-12 rounded-xl bg-white/[0.04] ring-1 ring-white/10 flex items-center justify-center text-[var(--color-faint)]">
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3" stroke-linecap="round"/></svg>
            </div>
            <p class="text-sm text-[var(--color-muted)]">No series match your filters.</p>
            {#if hasFilters}
              <Button variant="outline" size="sm" onclick={() => { searchQ=''; filterStatus=''; filterSubscribed=false; filterFavorite=false }}>Clear filters</Button>
            {/if}
          </div>
        {:else}
          <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-x-4 gap-y-6">
            {#each filtered as s (s.id)}
              <PosterCard series={s} />
            {/each}
          </div>
        {/if}
      </section>
    </div>
  {/if}
</div>

<!-- Add Series Modal -->
<Modal bind:open={addOpen} title="Add series">
  {#snippet footer()}
    <Button variant="ghost" onclick={() => { addOpen = false }}>Cancel</Button>
  {/snippet}

  <div class="space-y-4">
    <div class="flex gap-2">
      <Input bind:value={anilistQ} placeholder="Search AniList…" class="flex-1" />
      <Button onclick={searchAnilist} disabled={anilistLoading || !anilistQ.trim()}>
        {#if anilistLoading}<Spinner size={14}/>{:else}Search{/if}
      </Button>
    </div>

    {#if anilistResults.length > 0}
      <div class="space-y-2 max-h-80 overflow-y-auto -mx-1 px-1">
        {#each anilistResults as result (result.id)}
          <button
            class="w-full flex items-center gap-3 p-2.5 rounded-xl bg-[var(--color-surface-2)] hover:bg-[var(--color-surface-3)] transition-colors text-left cursor-pointer border border-transparent hover:border-[var(--accent)]/40"
            onclick={() => addSeries(result)}
            disabled={adding}
          >
            {#if result.cover_image}
              <img src={result.cover_image} alt="" loading="lazy" class="w-10 h-14 rounded-lg object-cover shrink-0" />
            {:else}
              <div class="w-10 h-14 rounded-lg bg-[var(--color-border)] shrink-0"></div>
            {/if}
            <div class="min-w-0 flex-1">
              <p class="text-[var(--color-text)] text-sm font-medium truncate">{result.english_title || result.romaji_title}</p>
              <p class="text-[var(--color-muted)] text-xs truncate">{result.romaji_title}</p>
              <p class="text-[var(--color-muted)] text-xs">{result.format} · {result.status} · {result.episode_count} ep</p>
            </div>
            {#if adding}
              <Spinner size={14}/>
            {:else}
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="var(--accent)" stroke-width="2.5" class="shrink-0"><path d="M12 5v14M5 12h14" stroke-linecap="round"/></svg>
            {/if}
          </button>
        {/each}
      </div>
    {:else if anilistQ && !anilistLoading}
      <p class="text-center text-[var(--color-muted)] text-sm py-4">No results. Try a different search.</p>
    {/if}
  </div>
</Modal>
