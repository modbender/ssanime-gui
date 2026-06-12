<script lang="ts">
  let { size = 20, class: cls = '' }: { size?: number; class?: string } = $props()
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  class="animate-spin {cls}"
  aria-label="Loading"
>
  <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="2.25" stroke-linecap="round" stroke-dasharray="40" stroke-dashoffset="10" opacity="0.2" />
  <path d="M12 2a10 10 0 0 1 10 10" stroke="currentColor" stroke-width="2.25" stroke-linecap="round" />
</svg>
