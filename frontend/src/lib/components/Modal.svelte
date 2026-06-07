<script lang="ts">
  let {
    open = $bindable(false),
    title = '',
    children,
    footer,
  }: {
    open?: boolean
    title?: string
    children?: any
    footer?: any
  } = $props()

  function close() { open = false }
  function onBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) close()
  }
  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') close()
  }
</script>

{#if open}
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  role="dialog"
  aria-modal="true"
  aria-label={title}
  class="fixed inset-0 z-50 flex items-center justify-center"
  onclick={onBackdropClick}
  onkeydown={onKeydown}
>
  <!-- Backdrop -->
  <div class="absolute inset-0 bg-black/70 backdrop-blur-sm"></div>

  <!-- Panel -->
  <div class="relative w-full max-w-lg mx-4 bg-[#111118] border border-[#2a2a35] rounded-2xl shadow-2xl">
    <div class="flex items-center justify-between px-5 py-4 border-b border-[#2a2a35]">
      <h2 class="text-base font-semibold text-[#e8e8f0]">{title}</h2>
      <button
        onclick={close}
        class="text-[#6b6b80] hover:text-[#e8e8f0] transition-colors rounded p-0.5"
        aria-label="Close"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M18 6L6 18M6 6l12 12"/></svg>
      </button>
    </div>
    <div class="px-5 py-4 max-h-[70vh] overflow-y-auto">
      {@render children?.()}
    </div>
    {#if footer}
    <div class="px-5 py-4 border-t border-[#2a2a35] flex justify-end gap-2">
      {@render footer?.()}
    </div>
    {/if}
  </div>
</div>
{/if}
