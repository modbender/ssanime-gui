<script lang="ts">
  import { api, type SeriesProgress } from '$lib/api'
  import Input from '$lib/components/Input.svelte'
  import Button from '$lib/components/Button.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import PosterCard from '$lib/components/PosterCard.svelte'
  import { errMessage, watchBucket, watchStatusLabel } from '$lib/utils'
  import { navigate } from 'svelte-routing'
  import { scrollScrim } from '$lib/scrollScrim'

  let all = $state<SeriesProgress[]>([])
  let loading = $state(true)
  let error = $state('')

  let searchQ = $state('')
  type Filter = 'all' | 'watching' | 'downloaded' | 'on_hold' | 'dropped' | 'completed'
  let filter = $state<Filter>('all')

  async function load() {
    loading = true
    error = ''
    try {
      all = await api.listSeries({ library: true })
    } catch (e: unknown) {
      error = errMessage(e)
    } finally {
      loading = false
    }
  }

  $effect(() => { load() })

  function matchesSearch(s: SeriesProgress): boolean {
    if (!searchQ) return true
    const q = searchQ.toLowerCase()
    return (
      s.title.toLowerCase().includes(q) ||
      (s.english_title ?? '').toLowerCase().includes(q) ||
      (s.romaji_title ?? '').toLowerCase().includes(q)
    )
  }

  // Bucket every series exactly once: Completed (derived) wins, else watch status.
  function bucketOf(s: SeriesProgress) {
    return watchBucket(s)
  }

  const searched = $derived(all.filter(matchesSearch))

  const counts = $derived.by(() => {
    const c = { all: searched.length, watching: 0, downloaded: 0, on_hold: 0, dropped: 0, completed: 0 }
    for (const s of searched) c[bucketOf(s)]++
    return c
  })

  const filtered = $derived(
    filter === 'all' ? searched : searched.filter((s) => bucketOf(s) === filter),
  )

  const chips: { value: Filter; label: string }[] = [
    { value: 'all', label: 'All' },
    { value: 'watching', label: watchStatusLabel('watching') },
    { value: 'downloaded', label: watchStatusLabel('downloaded') },
    { value: 'on_hold', label: watchStatusLabel('on_hold') },
    { value: 'dropped', label: watchStatusLabel('dropped') },
    { value: 'completed', label: watchStatusLabel('completed') },
  ]
</script>

<div class="flex flex-col h-full overflow-y-auto" use:scrollScrim>
  <!-- Header -->
  <div class="sticky top-0 z-10 bg-transparent backdrop-blur-0 border-b border-transparent transition-[background-color,border-color,backdrop-filter] duration-300 [.scrolled_&]:bg-[var(--color-bg)]/85 [.scrolled_&]:backdrop-blur-md [.scrolled_&]:border-[var(--color-border)]">
    <div class="px-6 sm:px-10 py-5 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div class="flex items-baseline gap-2.5">
        <h1 class="text-xl font-bold tracking-tight">Library</h1>
        <span class="text-xs font-medium text-[var(--color-muted)] tabular-nums">{all.length}</span>
      </div>
      <div class="relative">
        <svg class="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3" stroke-linecap="round"/></svg>
        <Input bind:value={searchQ} placeholder="Search library…" class="w-56 pl-9" />
      </div>
    </div>

    {#if !loading && all.length > 0}
      <div class="px-6 sm:px-10 pb-3 flex flex-wrap gap-2">
        {#each chips as chip (chip.value)}
          {@const on = filter === chip.value}
          <button
            type="button"
            aria-pressed={on}
            class="inline-flex items-center gap-1.5 border px-3 py-1 text-xs font-medium transition-colors
              {on
                ? 'border-[var(--accent-text)] bg-[rgb(var(--accent-rgb)/0.14)] text-[var(--color-text)]'
                : 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-border-strong)] hover:text-[var(--color-text)]'}"
            onclick={() => (filter = chip.value)}
          >
            {chip.label}
            <span class="tabular-nums text-[var(--color-faint)]">{counts[chip.value]}</span>
          </button>
        {/each}
      </div>
    {/if}
  </div>

  {#if loading}
    <div class="flex flex-1 items-center justify-center text-[var(--color-muted)]">
      <Spinner size={30} />
    </div>
  {:else if error}
    <div class="flex flex-1 items-center justify-center text-[var(--color-error)] text-sm">{error}</div>
  {:else if all.length === 0}
    <!-- Nothing subscribed yet — point at discovery -->
    <div class="flex flex-1 flex-col items-center justify-center gap-5 px-6 text-center">
      <div class="w-16 h-16 bg-white/[0.04] ring-1 ring-white/10 flex items-center justify-center text-[var(--color-faint)]">
        <svg width="30" height="30" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.25"><rect x="3" y="3" width="18" height="18" rx="3"/><path d="m3 16 5-5 4 4 3-3 6 6" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </div>
      <div class="space-y-1.5">
        <h2 class="text-lg font-semibold tracking-tight">Your library is empty</h2>
        <p class="text-sm text-[var(--color-muted)] max-w-sm">Find a series on the home page and hit Subscribe — it shows up here and auto-fetches every new episode.</p>
      </div>
      <Button size="lg" onclick={() => navigate('/')}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 10.5 12 3l9 7.5M5 9v10a1 1 0 0 0 1 1h4v-6h4v6h4a1 1 0 0 0 1-1V9" stroke-linecap="round" stroke-linejoin="round"/></svg>
        Browse discovery
      </Button>
    </div>
  {:else if filtered.length === 0}
    <div class="flex flex-1 items-center justify-center px-6 text-center text-sm text-[var(--color-muted)]">
      No series match this filter.
    </div>
  {:else}
    <div class="px-6 sm:px-10 py-8">
      <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-x-4 gap-y-6 animate-fade-up">
        {#each filtered as s (s.id)}
          <PosterCard series={s} showProgress={watchBucket(s) === 'watching' || watchBucket(s) === 'downloaded'} />
        {/each}
      </div>
    </div>
  {/if}
</div>
