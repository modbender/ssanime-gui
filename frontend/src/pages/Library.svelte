<script lang="ts">
  import { navigate } from 'svelte-routing'
  import { api, type SeriesProgress, type AnilistSearchResult } from '$lib/api'
  import Badge from '$lib/components/Badge.svelte'
  import Button from '$lib/components/Button.svelte'
  import Input from '$lib/components/Input.svelte'
  import Modal from '$lib/components/Modal.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import { derivedStatusColor, derivedStatusLabel, formatBytes } from '$lib/utils'

  let series = $state<SeriesProgress[]>([])
  let loading = $state(true)
  let error = $state('')

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
  }

  $effect(() => { load() })

  const filtered = $derived(series.filter(s => {
    if (searchQ && !s.title.toLowerCase().includes(searchQ.toLowerCase()) &&
        !(s.english_title ?? '').toLowerCase().includes(searchQ.toLowerCase())) return false
    if (filterStatus && s.derived_status !== filterStatus) return false
    if (filterSubscribed && !s.subscribed) return false
    if (filterFavorite && !s.favorite) return false
    return true
  }))

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
</script>

<div class="flex flex-col h-full">
  <!-- Header -->
  <div class="flex items-center justify-between px-6 py-4 border-b border-[#2a2a35]">
    <h1 class="text-lg font-semibold text-[#e8e8f0]">Library</h1>
    <div class="flex items-center gap-2">
      <Input bind:value={searchQ} placeholder="Search series…" class="w-52" />
      <select
        bind:value={filterStatus}
        class="h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] focus:outline-none focus:border-[#7c6af0] cursor-pointer"
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
        size="sm"
        onclick={() => { filterSubscribed = !filterSubscribed }}
      >Subscribed</Button>
      <Button
        variant={filterFavorite ? 'default' : 'outline'}
        size="sm"
        onclick={() => { filterFavorite = !filterFavorite }}
      >Favorites</Button>
      <Button onclick={() => { addOpen = true }}>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M12 5v14M5 12h14" stroke-linecap="round"/></svg>
        Add series
      </Button>
    </div>
  </div>

  <!-- Content -->
  <div class="flex-1 overflow-y-auto px-6 py-5">
    {#if loading}
      <div class="flex items-center justify-center h-64 text-[#6b6b80]">
        <Spinner size={28} />
      </div>
    {:else if error}
      <div class="flex items-center justify-center h-64 text-red-400 text-sm">{error}</div>
    {:else if filtered.length === 0}
      <div class="flex flex-col items-center justify-center h-64 gap-3">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="#2a2a35" stroke-width="1.5"><path d="M4 6a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6zm10 0a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2h-2a2 2 0 0 1-2-2V6zM4 16a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2v-2zm10 0a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2h-2a2 2 0 0 1-2-2v-2z"/></svg>
        <p class="text-[#6b6b80] text-sm">No series yet. Add one to get started.</p>
        <Button onclick={() => { addOpen = true }}>Add series</Button>
      </div>
    {:else}
      <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
        {#each filtered as s (s.id)}
          <button
            class="group text-left focus-visible:outline-2 focus-visible:outline-[#7c6af0] rounded-xl"
            onclick={() => navigate(`/series/${s.id}`)}
          >
            <!-- Poster -->
            <div class="relative aspect-[2/3] rounded-xl overflow-hidden bg-[#18181f] mb-2.5">
              {#if s.cover_image_url}
                <img
                  src={s.cover_image_url}
                  alt={s.title}
                  class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
                />
              {:else}
                <div class="w-full h-full flex items-center justify-center text-[#2a2a35]">
                  <svg width="40" height="40" viewBox="0 0 24 24" fill="currentColor"><path d="M4 6a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6z"/></svg>
                </div>
              {/if}
              <!-- Overlay badges -->
              <div class="absolute top-1.5 right-1.5 flex flex-col gap-1">
                {#if s.subscribed}
                  <span class="w-5 h-5 rounded-full bg-[#7c6af0]/90 flex items-center justify-center" title="Subscribed">
                    <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2.5"><path d="M15 17h5l-1.405-1.405A2.032 2.032 0 0 1 18 14.158V11a6.002 6.002 0 0 0-4-5.659V5a2 2 0 1 0-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 1 1-6 0v-1m6 0H9" stroke-linecap="round" stroke-linejoin="round"/></svg>
                  </span>
                {/if}
                {#if s.favorite}
                  <span class="w-5 h-5 rounded-full bg-yellow-500/90 flex items-center justify-center" title="Favorite">
                    <svg width="10" height="10" viewBox="0 0 24 24" fill="white" stroke="none"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>
                  </span>
                {/if}
              </div>
              <!-- Status badge bottom -->
              <div class="absolute bottom-0 left-0 right-0 p-1.5">
                <Badge class={derivedStatusColor(s.derived_status)}>
                  {derivedStatusLabel(s.derived_status)}
                </Badge>
              </div>
            </div>

            <!-- Meta -->
            <div class="space-y-0.5">
              <p class="text-[#e8e8f0] text-xs font-medium leading-tight line-clamp-2">{s.english_title || s.romaji_title || s.title}</p>
              <p class="text-[#6b6b80] text-xs">{s.episode_archived}/{s.episode_total} ep</p>
              {#if s.space_saved_bytes > 0}
                <p class="text-[#22c55e] text-xs">{formatBytes(s.space_saved_bytes)} saved</p>
              {/if}
            </div>
          </button>
        {/each}
      </div>
    {/if}
  </div>
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
      <div class="space-y-2 max-h-80 overflow-y-auto">
        {#each anilistResults as result (result.id)}
          <button
            class="w-full flex items-center gap-3 p-2.5 rounded-lg bg-[#18181f] hover:bg-[#2a2a35] transition-colors text-left cursor-pointer border border-transparent hover:border-[#7c6af0]/30"
            onclick={() => addSeries(result)}
            disabled={adding}
          >
            {#if result.cover_image}
              <img src={result.cover_image} alt="" class="w-10 h-14 rounded object-cover shrink-0" />
            {:else}
              <div class="w-10 h-14 rounded bg-[#2a2a35] shrink-0"></div>
            {/if}
            <div class="min-w-0 flex-1">
              <p class="text-[#e8e8f0] text-sm font-medium truncate">{result.english_title || result.romaji_title}</p>
              <p class="text-[#6b6b80] text-xs">{result.romaji_title}</p>
              <p class="text-[#6b6b80] text-xs">{result.format} · {result.status} · {result.episode_count} ep</p>
            </div>
            {#if adding}
              <Spinner size={14}/>
            {:else}
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#7c6af0" stroke-width="2.5" class="shrink-0"><path d="M12 5v14M5 12h14" stroke-linecap="round"/></svg>
            {/if}
          </button>
        {/each}
      </div>
    {:else if anilistQ && !anilistLoading}
      <p class="text-center text-[#6b6b80] text-sm py-4">No results. Try a different search.</p>
    {/if}
  </div>
</Modal>
