<script lang="ts">
  import { navigate } from 'svelte-routing'
  import type { DiscoveryItem } from '$lib/api'
  import { resolveAccent, hexToRgbChannels, accentForeground, accentText, accentTextRgb, titleCase } from '$lib/utils'
  import { rememberPreview, trackedAnilistIds } from '$lib/discovery.svelte'
  import Spinner from '$lib/components/Spinner.svelte'

  let {
    item,
    onTrack,
    tracking = false,
  }: {
    item: DiscoveryItem
    /** invoked on "Download & track"; parent owns the optimistic flow */
    onTrack?: (item: DiscoveryItem) => void
    /** true while a track request for this item is in flight */
    tracking?: boolean
  } = $props()

  const title = $derived(item.english_title || item.romaji_title)
  const accent = $derived(resolveAccent(item.cover_color))
  const accentRgb = $derived(hexToRgbChannels(item.cover_color))
  const accentFg = $derived(accentForeground(item.cover_color))
  const accentTxt = $derived(accentText(item.cover_color))
  const accentTxtRgb = $derived(accentTextRgb(item.cover_color))
  const tracked = $derived(trackedAnilistIds.has(item.anilist_id))

  function open() {
    rememberPreview(item)
    navigate(`/series/anilist/${item.anilist_id}`)
  }

  function track(e: MouseEvent) {
    e.stopPropagation()
    if (tracking || tracked) return
    onTrack?.(item)
  }
</script>

<div
  class="group block text-left w-full"
  style="--accent: {accent}; --accent-rgb: {accentRgb}; --accent-fg: {accentFg}; --accent-text: {accentTxt}; --accent-text-rgb: {accentTxtRgb};"
>
  <!-- Poster (clickable region; not a <button> so the CTA button can nest) -->
  <div
    role="button"
    tabindex="0"
    onclick={open}
    onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); open() } }}
    class="relative aspect-[2/3] w-full cursor-pointer overflow-hidden bg-[var(--color-surface-2)] ring-1 ring-white/[0.06]
           transition-[transform,box-shadow] duration-500 ease-[cubic-bezier(0.32,0.72,0,1)]
           group-hover:-translate-y-1.5 group-hover:shadow-[0_22px_45px_-18px_rgba(0,0,0,0.85)] group-hover:ring-white/15"
    aria-label={`Open ${title}`}
  >
    {#if item.cover_image}
      <img
        src={item.cover_image}
        alt={title}
        loading="lazy"
        class="w-full h-full object-cover transition-transform duration-700 ease-[cubic-bezier(0.32,0.72,0,1)] group-hover:scale-[1.07]"
      />
    {:else}
      <div
        class="w-full h-full flex items-center justify-center text-[var(--color-faint)]"
        style="background: radial-gradient(120% 120% at 50% 0%, rgb(var(--accent-rgb) / 0.28), var(--color-surface-2));"
      >
        <svg width="38" height="38" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.25"><rect x="3" y="3" width="18" height="18" rx="3"/><path d="m3 16 5-5 4 4 3-3 6 6" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </div>
    {/if}

    <!-- bottom scrim -->
    <div class="absolute inset-x-0 bottom-0 h-3/5 bg-gradient-to-t from-black/90 via-black/30 to-transparent transition-opacity duration-300 opacity-80 group-hover:opacity-95"></div>

    <!-- tracked check (top-right) -->
    {#if tracked}
      <span class="absolute top-2 right-2 w-6 h-6 bg-[var(--color-success)] shadow-lg flex items-center justify-center" title="Tracking">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="#062018" stroke-width="2.75"><path d="M20 6 9 17l-5-5" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </span>
    {/if}

    <!-- format tag (top-left) -->
    {#if item.format}
      <span class="absolute top-2 left-2 bg-black/55 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-white/85 backdrop-blur-sm ring-1 ring-white/10">
        {titleCase(item.format)}
      </span>
    {/if}

    <!-- hover CTA: Download & track -->
    <div class="absolute inset-x-0 bottom-0 p-2 translate-y-1 opacity-0 transition-all duration-300 ease-[cubic-bezier(0.32,0.72,0,1)] group-hover:translate-y-0 group-hover:opacity-100">
      {#if tracked}
        <div class="flex h-8 w-full items-center justify-center gap-1.5 bg-[var(--color-success)]/90 text-[12px] font-semibold text-[#062018]">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M20 6 9 17l-5-5" stroke-linecap="round" stroke-linejoin="round"/></svg>
          Tracking
        </div>
      {:else}
        <button
          type="button"
          onclick={track}
          disabled={tracking}
          class="flex h-8 w-full items-center justify-center gap-1.5 bg-[var(--accent)] text-[12px] font-semibold text-[var(--accent-fg)] shadow-[0_4px_18px_-6px_rgb(var(--accent-rgb)/0.8)] transition-[filter,transform] duration-200 hover:brightness-110 active:scale-[0.97] disabled:opacity-70 disabled:active:scale-100 cursor-pointer"
        >
          {#if tracking}
            <Spinner size={13} />
            Tracking…
          {:else}
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25"><path d="M12 3v12m0 0 4-4m-4 4-4-4M5 21h14" stroke-linecap="round" stroke-linejoin="round"/></svg>
            Subscribe
          {/if}
        </button>
      {/if}
    </div>
  </div>

  <!-- meta -->
  <div class="mt-2.5 px-0.5 space-y-0.5">
    <p class="text-[13px] font-medium leading-snug text-[var(--color-text)] line-clamp-1 transition-colors duration-200 group-hover:text-[var(--accent-text)]">
      {title}
    </p>
    <p class="text-[11px] text-[var(--color-muted)] truncate">
      {#if item.season_year}{titleCase(item.season)} {item.season_year}{:else}{titleCase(item.status) || 'Anime'}{/if}
      {#if item.episode_count}<span class="text-[var(--color-faint)]"> · {item.episode_count} ep</span>{/if}
    </p>
  </div>
</div>
