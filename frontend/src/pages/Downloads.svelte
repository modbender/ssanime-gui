<script lang="ts">
  import { api, type SeriesProgress } from '$lib/api'
  import Button from '$lib/components/Button.svelte'
  import Input from '$lib/components/Input.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import PosterCard from '$lib/components/PosterCard.svelte'
  import { trackedStatus } from '$lib/utils'
  import { navigate } from 'svelte-routing'

  let active = $state<SeriesProgress[]>([])
  let completed = $state<SeriesProgress[]>([])
  let paused = $state<SeriesProgress[]>([])
  let dropped = $state<SeriesProgress[]>([])
  let loading = $state(true)
  let error = $state('')

  let searchQ = $state('')
  // per-series action in flight (by id), to disable controls
  let busy = $state<Set<number>>(new Set())

  async function load() {
    loading = true
    error = ''
    try {
      const res = await api.getTracked()
      active = res.in_progress ?? []
      completed = res.completed ?? []
      paused = res.paused ?? []
      dropped = res.dropped ?? []
    } catch (e: any) {
      error = e.message
    } finally {
      loading = false
    }
  }

  $effect(() => { load() })

  function matches(s: SeriesProgress): boolean {
    if (!searchQ) return true
    const q = searchQ.toLowerCase()
    return (
      s.title.toLowerCase().includes(q) ||
      (s.english_title ?? '').toLowerCase().includes(q) ||
      (s.romaji_title ?? '').toLowerCase().includes(q)
    )
  }

  const sections = $derived([
    { key: 'active', label: 'Active', items: active.filter(matches) },
    { key: 'completed', label: 'Completed', items: completed.filter(matches) },
    { key: 'paused', label: 'Paused', items: paused.filter(matches) },
    { key: 'dropped', label: 'Dropped', items: dropped.filter(matches) },
  ])

  const total = $derived(active.length + completed.length + paused.length + dropped.length)

  async function setBusy(id: number, on: boolean) {
    const next = new Set(busy)
    if (on) next.add(id); else next.delete(id)
    busy = next
  }

  async function run(id: number, fn: () => Promise<unknown>) {
    if (busy.has(id)) return
    await setBusy(id, true)
    try {
      await fn()
      await load()
    } catch (e: any) {
      alert(e.message)
    } finally {
      await setBusy(id, false)
    }
  }

  const pause = (s: SeriesProgress) => run(s.id, () => api.pauseSeries(s.id))
  const drop = (s: SeriesProgress) => run(s.id, () => api.dropSeries(s.id))
  const resume = (s: SeriesProgress) => run(s.id, () => api.resumeSeries(s.id))
</script>

<div class="flex flex-col h-full overflow-y-auto">
  <!-- Header -->
  <div class="sticky top-0 z-10 bg-[var(--color-bg)]/85 backdrop-blur-md border-b border-[var(--color-border)]">
    <div class="px-6 sm:px-10 py-5 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div class="flex items-baseline gap-2.5">
        <h1 class="text-xl font-bold tracking-tight">Downloads</h1>
        <span class="text-xs font-medium text-[var(--color-muted)] tabular-nums">{total}</span>
      </div>
      <div class="relative">
        <svg class="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3" stroke-linecap="round"/></svg>
        <Input bind:value={searchQ} placeholder="Search downloads…" class="w-56 pl-9" />
      </div>
    </div>
  </div>

  {#if loading}
    <div class="flex flex-1 items-center justify-center text-[var(--color-muted)]">
      <Spinner size={30} />
    </div>
  {:else if error}
    <div class="flex flex-1 items-center justify-center text-[var(--color-error)] text-sm">{error}</div>
  {:else if total === 0}
    <!-- Nothing tracked yet — point to discovery, never the old "add your first series" -->
    <div class="flex flex-1 flex-col items-center justify-center gap-5 px-6 text-center">
      <div class="w-16 h-16 bg-white/[0.04] ring-1 ring-white/10 flex items-center justify-center text-[var(--color-faint)]">
        <svg width="30" height="30" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.25"><path d="M12 3v12m0 0 4-4m-4 4-4-4M5 21h14" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </div>
      <div class="space-y-1.5">
        <h2 class="text-lg font-semibold tracking-tight">No downloads yet</h2>
        <p class="text-sm text-[var(--color-muted)] max-w-sm">Find a series on the home page and hit “Download &amp; track” — it’ll show up here and auto-fetch every new episode.</p>
      </div>
      <Button size="lg" onclick={() => navigate('/')}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 10.5 12 3l9 7.5M5 9v10a1 1 0 0 0 1 1h4v-6h4v6h4a1 1 0 0 0 1-1V9" stroke-linecap="round" stroke-linejoin="round"/></svg>
        Browse discovery
      </Button>
    </div>
  {:else}
    <div class="px-6 sm:px-10 py-8 space-y-12">
      {#each sections as section (section.key)}
        {#if section.items.length > 0}
          <section class="animate-fade-up">
            <div class="flex items-baseline gap-2.5 mb-5">
              <h2 class="text-[15px] font-semibold tracking-tight">{section.label}</h2>
              <span class="text-xs font-medium text-[var(--color-muted)] tabular-nums">{section.items.length}</span>
            </div>
            <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-x-4 gap-y-6">
              {#each section.items as s (s.id)}
                {@const st = trackedStatus(s)}
                <div class="group/card">
                  <PosterCard series={s} showProgress={section.key === 'active'} />
                  <!-- Per-card controls -->
                  <div class="mt-2 flex items-center gap-1.5">
                    {#if st === 'paused' || st === 'dropped'}
                      <Button
                        variant="secondary"
                        size="sm"
                        class="flex-1"
                        disabled={busy.has(s.id)}
                        onclick={() => resume(s)}
                      >
                        {#if busy.has(s.id)}<Spinner size={12}/>{:else}
                          <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" stroke="none"><path d="M8 5v14l11-7z"/></svg>
                        {/if}
                        Resume
                      </Button>
                    {:else}
                      <Button
                        variant="outline"
                        size="sm"
                        class="flex-1"
                        disabled={busy.has(s.id)}
                        onclick={() => pause(s)}
                        title="Pause auto-download"
                      >
                        {#if busy.has(s.id)}<Spinner size={12}/>{:else}
                          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25"><path d="M6 4h3v16H6zM15 4h3v16h-3z"/></svg>
                        {/if}
                        Pause
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={busy.has(s.id)}
                        onclick={() => drop(s)}
                        title="Drop (keeps files)"
                      >
                        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6 6 18M6 6l12 12" stroke-linecap="round"/></svg>
                      </Button>
                    {/if}
                  </div>
                </div>
              {/each}
            </div>
          </section>
        {/if}
      {/each}
    </div>
  {/if}
</div>
