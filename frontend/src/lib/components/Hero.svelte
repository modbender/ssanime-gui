<script lang="ts">
  import { navigate } from 'svelte-routing'
  import type { SeriesProgress } from '$lib/api'
  import Button from '$lib/components/Button.svelte'
  import { derivedStatusColor, derivedStatusLabel, resolveAccent, hexToRgbChannels } from '$lib/utils'

  let {
    items = [] as SeriesProgress[],
    onToggleSubscribe,
  }: {
    items: SeriesProgress[]
    onToggleSubscribe?: (s: SeriesProgress) => void
  } = $props()

  let index = $state(0)
  const featured = $derived(items[index] ?? null)

  // cover_color → accent, with graceful violet fallback
  const accent = $derived(resolveAccent(featured?.cover_color))
  const accentRgb = $derived(hexToRgbChannels(accent))

  const title = $derived(
    featured ? (featured.english_title || featured.romaji_title || featured.title) : '',
  )
  const banner = $derived(featured?.banner_image_url || featured?.cover_image_url || null)

  // Autorotate when more than one featured series.
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

  function fmtAiring(s: string | null): string | null {
    if (!s) return null
    return s.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
  }
</script>

{#if featured}
  <section
    class="relative w-full overflow-hidden flex items-end min-h-[72vh] shrink-0"
    style="--accent: {accent}; --accent-rgb: {accentRgb};"
  >
    <!-- Banner layer -->
    <div class="absolute inset-0">
      {#if banner}
        {#key featured.id}
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
          <span class="inline-flex items-center gap-1.5 rounded-full bg-white/5 ring-1 ring-white/10 px-3 py-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-[var(--color-text-dim)]">
            <span class="w-1.5 h-1.5 rounded-full bg-[var(--accent)]"></span>
            Featured
          </span>
          {#if featured.feed_title}
            <span class="text-[11px] text-[var(--color-muted)]">via {featured.feed_title}</span>
          {/if}
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
            <span class="inline-flex items-center rounded-full bg-white/[0.06] ring-1 ring-white/10 px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)]">{featured.format}</span>
          {/if}
          <span class={`inline-flex items-center rounded-full border px-2.5 py-1 text-[11px] font-medium backdrop-blur-sm ${derivedStatusColor(featured.derived_status)}`}>
            {derivedStatusLabel(featured.derived_status)}
          </span>
          {#if fmtAiring(featured.airing_status)}
            <span class="inline-flex items-center rounded-full bg-white/[0.06] ring-1 ring-white/10 px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)]">{fmtAiring(featured.airing_status)}</span>
          {/if}
          <span class="inline-flex items-center gap-1.5 rounded-full bg-white/[0.06] ring-1 ring-white/10 px-2.5 py-1 text-[11px] font-medium text-[var(--color-text-dim)] tabular-nums">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 9h18" stroke-linecap="round"/></svg>
            {featured.episode_archived}/{featured.episode_total} archived
          </span>
        </div>

        <!-- context line -->
        <p class="mt-4 text-sm leading-relaxed text-[var(--color-text-dim)] max-w-xl">
          {#if featured.subscribed}
            You're subscribed — new episodes auto-download and re-encode as they air.
          {:else}
            Subscribe to auto-fetch and durably re-encode every new episode into your library.
          {/if}
        </p>

        <!-- actions -->
        <div class="mt-7 flex items-center gap-2.5">
          <Button size="lg" onclick={() => navigate(`/series/${featured.id}`)}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 3v18l15-9z" fill="currentColor" stroke="none"/></svg>
            View series
          </Button>
          <Button
            size="lg"
            variant={featured.subscribed ? 'secondary' : 'outline'}
            onclick={() => onToggleSubscribe?.(featured)}
          >
            {#if featured.subscribed}
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 6 9 17l-5-5" stroke-linecap="round" stroke-linejoin="round"/></svg>
              Subscribed
            {:else}
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9M13.7 21a2 2 0 0 1-3.4 0" stroke-linecap="round" stroke-linejoin="round"/></svg>
              Subscribe
            {/if}
          </Button>
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
