<script lang="ts">
  import { cn } from '$lib/utils'

  let {
    value = $bindable(''),
    placeholder = '',
    type = 'text',
    disabled = false,
    class: cls = '',
    oninput,
    onchange,
  }: {
    value?: string
    placeholder?: string
    type?: string
    disabled?: boolean
    class?: string
    oninput?: (e: Event) => void
    onchange?: (e: Event) => void
  } = $props()
</script>

<input
  {type}
  bind:value
  {placeholder}
  {disabled}
  oninput={oninput}
  onchange={onchange}
  class={cn(
    'w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)]',
    'placeholder:text-[var(--color-muted)] transition-colors duration-200',
    'focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)] disabled:opacity-50',
    cls,
  )}
/>
