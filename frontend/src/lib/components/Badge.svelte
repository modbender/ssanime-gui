<script lang="ts">
  import { cn } from '$lib/utils'
  import type { Snippet } from 'svelte'
  let { class: cls = '', children }: { class?: string; children?: Snippet } = $props()
</script>

<span class={cn(
  'inline-flex items-center gap-1 border px-2.5 py-0.5 text-[11px] font-medium leading-none tracking-tight backdrop-blur-sm',
  cls
)}>
  {@render children?.()}
</span>
