<script lang="ts">
  import { cn } from '$lib/utils'
  let { class: cls = '', children }: { class?: string; children?: any } = $props()
</script>

<span class={cn(
  'inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium',
  cls
)}>
  {@render children?.()}
</span>
