<script lang="ts">
  import type { Snippet } from 'svelte'
  let {
    title,
    count = 0,
    children,
  }: {
    title: string
    count?: number
    children?: Snippet
  } = $props()

  let scroller = $state<HTMLDivElement | null>(null)

  function scrollBy(dir: number) {
    scroller?.scrollBy({ left: dir * (scroller.clientWidth * 0.8), behavior: 'smooth' })
  }
</script>

<section class="animate-fade-up">
  <div class="flex items-end justify-between mb-3.5 px-0.5">
    <div class="flex items-baseline gap-2.5">
      <h2 class="text-[15px] font-semibold tracking-tight text-[var(--color-text)]">{title}</h2>
      {#if count > 0}
        <span class="text-xs font-medium text-[var(--color-muted)] tabular-nums">{count}</span>
      {/if}
    </div>
    <div class="flex gap-1.5">
      <button
        onclick={() => scrollBy(-1)}
        aria-label="Scroll left"
        class="w-8 h-8 flex items-center justify-center text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-white/5 transition-colors duration-200"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m15 18-6-6 6-6" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </button>
      <button
        onclick={() => scrollBy(1)}
        aria-label="Scroll right"
        class="w-8 h-8 flex items-center justify-center text-[var(--color-muted)] hover:text-[var(--color-text)] hover:bg-white/5 transition-colors duration-200"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m9 18 6-6-6-6" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </button>
    </div>
  </div>

  <div
    bind:this={scroller}
    class="no-scrollbar flex gap-4 overflow-x-auto pb-2 -mx-1 px-1 scroll-px-1 snap-x"
    style="scroll-snap-type: x proximity;"
  >
    {@render children?.()}
  </div>
</section>
