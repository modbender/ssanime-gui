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
    'w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] placeholder:text-[#6b6b80] focus:outline-none focus:border-[#7c6af0] transition-colors disabled:opacity-50',
    cls,
  )}
/>
