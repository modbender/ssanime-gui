<script lang="ts">
  let {
    value = 0,
    max = 100,
    class: cls = '',
    color = 'bg-[#7c6af0]',
  }: {
    value?: number
    max?: number
    class?: string
    color?: string
  } = $props()

  const pct = $derived(Math.min(100, Math.round((value / Math.max(max, 1)) * 100)))
</script>

<div class="w-full bg-[#2a2a35] rounded-full h-1.5 overflow-hidden {cls}" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}>
  <div class="{color} h-full rounded-full transition-all duration-300" style="width: {pct}%"></div>
</div>
