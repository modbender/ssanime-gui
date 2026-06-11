<script lang="ts">
  let { title = '', count = 7 }: { title?: string; count?: number } = $props()
</script>

<section class="animate-fade-up">
  <div class="flex items-end justify-between mb-3.5 px-0.5">
    <div class="flex items-baseline gap-2.5">
      {#if title}
        <h2 class="text-[15px] font-semibold tracking-tight text-[var(--color-text)]">{title}</h2>
      {:else}
        <div class="h-4 w-40 rounded-md bg-white/[0.06] animate-pulse"></div>
      {/if}
    </div>
  </div>

  <div class="flex gap-4 overflow-hidden pb-2 -mx-1 px-1">
    {#each Array(count) as _, i (i)}
      <div class="shrink-0 w-[150px]">
        <div class="aspect-[2/3] w-full rounded-[var(--radius-card)] bg-white/[0.05] ring-1 ring-white/[0.04] animate-pulse"></div>
        <div class="mt-2.5 space-y-1.5 px-0.5">
          <div class="h-3 w-[85%] rounded bg-white/[0.05] animate-pulse"></div>
          <div class="h-2.5 w-[55%] rounded bg-white/[0.04] animate-pulse"></div>
        </div>
      </div>
    {/each}
  </div>
</section>
