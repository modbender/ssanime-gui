<script lang="ts">
  import { cn } from '$lib/utils'
  let { class: cls = '', children }: { class?: string; children?: any } = $props()
</script>

<span class={cn(
  'inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-[11px] font-medium leading-none tracking-tight backdrop-blur-sm',
  cls
)}>
  {@render children?.()}
</span>
