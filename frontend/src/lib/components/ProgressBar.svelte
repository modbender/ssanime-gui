<script lang="ts">
  let {
    value = 0,
    max = 100,
    class: cls = '',
    color = 'bg-[var(--accent-text)]',
  }: {
    value?: number
    max?: number
    class?: string
    color?: string
  } = $props()

  const pct = $derived(Math.min(100, Math.round((value / Math.max(max, 1)) * 100)))
</script>

<div class="w-full bg-white/8 h-1.5 overflow-hidden {cls}" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}>
  <div class="{color} h-full transition-[width] duration-500 ease-[cubic-bezier(0.32,0.72,0,1)]" style="width: {pct}%"></div>
</div>
