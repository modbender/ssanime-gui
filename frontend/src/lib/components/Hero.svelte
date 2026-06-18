<script lang="ts">
  import { navigate } from 'svelte-routing'
  import type { DiscoveryItem } from '$lib/api'
  import Button from '$lib/components/Button.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import { resolveAccent, hexToRgbChannels, accentForeground, accentText, accentTextRgb, titleCase } from '$lib/utils'
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
  // pass = number of completed carousel loops; advances the per-series wide-image
  // variant so a title shows a different banner each time it comes back around
  // (Loop 1 → image[0], Loop 2 → image[1], …).
  let pass = $state(0)
  // Random per-open offset so repeated app-opens start on a different image.
  const seed = Math.floor(Math.random() * 997)
  const featured = $derived(items[index] ?? null)

  const accent = $derived(resolveAccent(featured?.cover_color))
  const accentRgb = $derived(hexToRgbChannels(featured?.cover_color))
  const accentFg = $derived(accentForeground(featured?.cover_color))
  const accentTxt = $derived(accentText(featured?.cover_color))
  const accentTxtRgb = $derived(accentTextRgb(featured?.cover_color))

  const title = $derived(featured ? featured.english_title || featured.romaji_title : '')
  // Hero art pool: the AniList banner first (reliably hero-sized), then the wide
  // ani.zip fanart variants for per-loop rotation. Never the portrait cover_image
  // — stretched across the hero it reads soft/low-res; with no wide art we render
  // the accent gradient instead.
  const widePool = $derived.by(() => {
    const seen = new Set<string>()
    for (const u of [featured?.banner_image, ...(featured?.wide_images ?? [])]) {
      if (u) seen.add(u)
    }
    return [...seen]
  })
  const banner = $derived(widePool.length ? widePool[(seed + pass) % widePool.length] : null)
  const logo = $derived(featured?.clear_logo_url || '')
  const tracked = $derived(featured ? trackedAnilistIds.has(featured.anilist_id) : false)
  const tracking = $derived(featured ? trackingId === featured.anilist_id : false)

  // A clearLogo PNG can 404; fall back to the text title when it does. Reset the
  // flag whenever the slide changes so one bad logo doesn't suppress the next.
  let logoFailed = $state(false)
  $effect(() => {
    void featured?.anilist_id
    logoFailed = false
  })
  const showLogo = $derived(logo !== '' && !logoFailed)

  // Autorotate when more than one featured item; bump `pass` on each full loop so
  // the wide-image variant advances when the carousel wraps back to the start.
  $effect(() => {
    if (items.length <= 1) return
    const id = setInterval(() => {
      const next = (index + 1) % items.length
      index = next
      if (next === 0) pass++
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
    class="relative -ml-[var(--rail)] w-[calc(100%+var(--rail))] overflow-hidden flex items-end min-h-[72vh] shrink-0"
    style="--accent: {accent}; --accent-rgb: {accentRgb}; --accent-fg: {accentFg}; --accent-text: {accentTxt}; --accent-text-rgb: {accentTxtRgb};"
  >
    <!-- Banner layer -->
    <div class="absolute inset-0">
      {#if banner}
        {#key banner}
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

    <!-- Logo-forward: keep ONLY a subtle bottom-up fade under the lower controls so the
         chips/buttons/light logo stay legible. The banner art reads largely un-darkened.
         A short solid baseline seams the hero into the page below it. -->
    <div
      class="absolute inset-x-0 bottom-0 h-[50%]"
      style="background: linear-gradient(to top, var(--color-bg) 0%, rgb(8 8 11 / 0.55) 28%, rgb(8 8 11 / 0.12) 60%, transparent 100%);"
    ></div>

    <!-- Content — left padding clears the floating rail; art behind it stays full-bleed -->
    <div class="relative w-full pr-6 sm:pr-10 pl-[calc(var(--rail)+1.5rem)] sm:pl-[calc(var(--rail)+2.5rem)] pt-32 pb-12 max-w-[1500px]">
      <div class="max-w-2xl animate-fade-up">
        <!-- eyebrow -->
        <div class="flex items-center gap-2 mb-4">
          <span class="inline-flex items-center gap-1.5 bg-black/55 backdrop-blur-sm ring-1 ring-white/15 px-3 py-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-white/90">
            <span class="w-1.5 h-1.5 rounded-full bg-[var(--accent-text)]"></span>
            Trending now
          </span>
        </div>

        <!-- title: transparent clearLogo art when available, styled text otherwise -->
        {#if showLogo}
          {#key featured.anilist_id}
            <img
              src={logo}
              alt={title}
              onerror={() => (logoFailed = true)}
              class="block w-auto max-w-[clamp(220px,42vw,560px)] max-h-[clamp(88px,18vh,168px)] object-contain object-left-bottom origin-left animate-fade-up drop-shadow-[0_4px_24px_rgba(0,0,0,0.55)]"
            />
          {/key}
        {:else}
          <h1 class="text-4xl sm:text-5xl font-extrabold tracking-tight leading-[1.04] text-white drop-shadow-[0_2px_20px_rgba(0,0,0,0.6)]">
            {title}
          </h1>
          {#if featured.romaji_title && featured.romaji_title !== title}
            <p class="mt-1.5 text-sm text-[var(--color-text-dim)]">{featured.romaji_title}</p>
          {/if}
        {/if}

        <!-- meta chips -->
        <div class="mt-5 flex flex-wrap items-center gap-2">
          {#if featured.format}
            <span class="inline-flex items-center bg-black/55 backdrop-blur-sm ring-1 ring-white/15 px-2.5 py-1 text-[11px] font-medium text-white/90">{titleCase(featured.format)}</span>
          {/if}
          {#if featured.status}
            <span class="inline-flex items-center bg-black/55 backdrop-blur-sm ring-1 ring-white/15 px-2.5 py-1 text-[11px] font-medium text-white/90">{titleCase(featured.status)}</span>
          {/if}
          {#if featured.season_year}
            <span class="inline-flex items-center bg-black/55 backdrop-blur-sm ring-1 ring-white/15 px-2.5 py-1 text-[11px] font-medium text-white/90">{titleCase(featured.season)} {featured.season_year}</span>
          {/if}
          {#if featured.episode_count}
            <span class="inline-flex items-center gap-1.5 bg-black/55 backdrop-blur-sm ring-1 ring-white/15 px-2.5 py-1 text-[11px] font-medium text-white/90 tabular-nums">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 9h18" stroke-linecap="round"/></svg>
              {featured.episode_count} episodes
            </span>
          {/if}
        </div>

        <!-- actions -->
        <div class="mt-7 flex items-center gap-2.5">
          {#if tracked}
            <Button size="lg" variant="secondary" onclick={open}>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 6 9 17l-5-5" stroke-linecap="round" stroke-linejoin="round"/></svg>
              Subscribed — view series
            </Button>
          {:else}
            <Button size="lg" onclick={() => onTrack?.(featured)} disabled={tracking}>
              {#if tracking}
                <Spinner size={16} />
                Subscribing…
              {:else}
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 3v12m0 0 4-4m-4 4-4-4M5 21h14" stroke-linecap="round" stroke-linejoin="round"/></svg>
                Subscribe
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
                ? 'w-7 bg-[var(--accent-text)]'
                : 'w-1.5 bg-white/25 hover:bg-white/40'}"
            ></button>
          {/each}
        </div>
      {/if}
    </div>
  </section>
{/if}
