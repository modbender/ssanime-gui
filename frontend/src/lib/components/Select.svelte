<script lang="ts">
  import { cn } from '$lib/utils'

  let {
    value = $bindable(''),
    options = [] as { label: string; value: string | number }[],
    placeholder = 'Select…',
    disabled = false,
    class: cls = '',
    onchange,
  }: {
    value?: string | number
    options?: { label: string; value: string | number }[]
    placeholder?: string
    disabled?: boolean
    class?: string
    onchange?: (e: Event) => void
  } = $props()
</script>

<select
  bind:value
  {disabled}
  onchange={onchange}
  class={cn(
    'w-full h-9 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)]',
    'transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] disabled:opacity-50 cursor-pointer',
    cls,
  )}
>
  {#if placeholder}
    <option value="" disabled selected={!value}>{placeholder}</option>
  {/if}
  {#each options as opt}
    <option value={opt.value}>{opt.label}</option>
  {/each}
</select>
