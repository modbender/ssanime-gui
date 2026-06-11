<script lang="ts">
  import { navigate } from 'svelte-routing'
  import type { DiscoveryItem } from '$lib/api'
  import Button from '$lib/components/Button.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import { resolveAccent, hexToRgbChannels, accentForeground, titleCase } from '$lib/utils'
  import { rememberPreview, trackedAnilistIds } from '$lib/discovery.svelte'

  let {
    items = [] as DiscoveryItem[],
    onTrack,
    trackingId = null,
  }: {
    items: DiscoveryItem[]
    /** invoked on "Download & track"; parent owns the optimistic flow */
    onTrack?: (item: DiscoveryItem) => void
    /** anilist_id currently being tracked (spinner state), or null */
    trackingId?: number | null
  } = $props()

  let index = $state(0)
  const featured = $derived(items[index] ?? null)

  const accent = $derived(resolveAccent(featured?.cover_color))
  const accentRgb = $derived(hexToRgbChannels(featured?.cover_color))
  const accentFg = $derived(accentForeground(featured?.cover_color))

  const title = $derived(featured ? featured.english_title || featured.romaji_title : '')
  const banner = $derived(featured?.banner_image || featured?.cover_image || null)
  const tracked = $derived(featured ? trackedAnilistIds.has(featured.anilist_id) : false)
  const tracking = $derived(featured ? trackingId === featured.anilist_id : false)

  // Autorotate when more than one featured item.
  $effect(() => {
    if (items.length <= 1) return
    const id = setInterval(() => {
      index = (index + 1) % items.length
    }, 8000)
    return () => clearInterval(id)
  })

  // Keep index valid if the list shrinks.
  $effect(() => {
    if (index >= items.length) index = 0
  })

  function open() {
    if (!featured) return
    rememberPreview(featured)
    navigate(`/series/anilist/${featured.anilist_id}`)
  }
</script>

{#if featured}
  <section
    class="relative w-full overflow-hidden flex items-end min-h-[72vh] shrink-0"
    style="--accent: {accent}; --accent-rgb: {accentRgb}; --accent-fg: {accentFg};"
  >
    <!-- Banner layer -->
    <div class="absolute inset-0">
      {#if banner}
        {#key featured.anilist_id}
          <img
            src={banner}
            alt=""
            class="w-full h-full object-cover object-center animate-fade"
            style="animation-duration:.9s"
          />
        {/key}
      {:else}
        <div
          class="w-full h-full"
          style="background:
            radial-gradient(120% 100% at 80% 0%, rgb(var(--accent-rgb) / 0.35), transparent 60%),
            radial-gradient(100% 100% at 0% 100%, rgb(var(--accent-rgb) / 0.18), transparent 55%),
            var(--color-surface);"
        ></div>
      {/if}
    </div>

    <!-- Scrims: bottom fade into page, left fade for text legibility, accent tint -->
    <div class="absolute inset-0 bg-gradient-to-t from-[var(--color-bg)] via-[var(--color-bg)]/55 to-transparent"></div>
    <div class="absolute inset-0 bg-gradient-to-r from-[var(--color-bg)] via-[var(--color-bg)]/60 to-transparent"></div>
    <div
      class="absolute inset-0 mix-blend-soft-light opacity-60"
      style="background: radial-gradient(90% 120% at 10% 100%, rgb(var(--accent-rgb) / 0.55), transparent 60%);"
    ></div>

    <!-- Content -->
    <div class="relative w-full px-6 sm:px-10 pt-32 pb-12 max-w-[1500px]">
      <div class="max-w-2xl animate-fade-up">
        <!-- eyebrow -->
        <div class="flex items-center gap-2 mb-4">
          <span class="inline-flex items-center gap-1.5 bg-white/5 ring-1 ring-white/10 px-3 py-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-[var(--color-text-dim)]">
            <span class="w-1.5 h-1.5 rounded-full bg-[var(--accent)]"></span>
            Trending now
          </span>
        </div>

        <!-- title -->
        <h1 class="text-4xl sm:text-5xl font-extrabold tracking-tight leading-[1.04] text-white drop-shadow-[0_2px_20px_rgba(0,0,0,0.6)]">
          {title}
        </h1>
        {#if featured.romaji_title && featured.romaji_title !== title}
          <p class="mt-1.5 text-sm text-[var(--color-text-dim)]">{featured.romaji_title}</p>
        {/if}

        <!-- meta chips -->
        <div class="mt-5 flex flex-wrap items-center gap-2">
          {#if featured.format}
            <span class="inline-flex items-center bg-white/[0.06] ring-1 ring-white/10 px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)]">{titleCase(featured.format)}</span>
          {/if}
          {#if featured.status}
            <span class="inline-flex items-center bg-white/[0.06] ring-1 ring-white/10 px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)]">{titleCase(featured.status)}</span>
          {/if}
          {#if featured.season_year}
            <span class="inline-flex items-center bg-white/[0.06] ring-1 ring-white/10 px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)]">{titleCase(featured.season)} {featured.season_year}</span>
          {/if}
          {#if featured.episode_count}
            <span class="inline-flex items-center gap-1.5 bg-white/[0.06] ring-1 ring-white/10 px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)] tabular-nums">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 9h18" stroke-linecap="round"/></svg>
              {featured.episode_count} episodes
            </span>
          {/if}
        </div>

        <!-- context line -->
        <p class="mt-4 text-sm leading-relaxed text-[var(--color-text-dim)] max-w-xl">
          Download &amp; track to auto-fetch and durably re-encode every episode into your library as it airs.
        </p>

        <!-- actions -->
        <div class="mt-7 flex items-center gap-2.5">
          {#if tracked}
            <Button size="lg" variant="secondary" onclick={open}>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 6 9 17l-5-5" stroke-linecap="round" stroke-linejoin="round"/></svg>
              Tracking — view series
            </Button>
          {:else}
            <Button size="lg" onclick={() => onTrack?.(featured)} disabled={tracking}>
              {#if tracking}
                <Spinner size={16} />
                Tracking…
              {:else}
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 3v12m0 0 4-4m-4 4-4-4M5 21h14" stroke-linecap="round" stroke-linejoin="round"/></svg>
                Download &amp; track
              {/if}
            </Button>
            <Button size="lg" variant="outline" onclick={open}>
              View details
            </Button>
          {/if}
        </div>
      </div>

      <!-- carousel dots -->
      {#if items.length > 1}
        <div class="mt-8 flex items-center gap-2">
          {#each items as _, i}
            <button
              aria-label={`Show featured ${i + 1}`}
              onclick={() => (index = i)}
              class="h-1.5 rounded-full transition-all duration-500 ease-[cubic-bezier(0.32,0.72,0,1)] {i === index
                ? 'w-7 bg-[var(--accent)]'
                : 'w-1.5 bg-white/25 hover:bg-white/40'}"
            ></button>
          {/each}
        </div>
      {/if}
    </div>
  </section>
{/if}
