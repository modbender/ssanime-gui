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
  tabindex="-1"
  class="fixed inset-0 z-50 flex items-center justify-center animate-fade"
  onclick={onBackdropClick}
  onkeydown={onKeydown}
>
  <!-- Backdrop -->
  <div class="absolute inset-0 bg-black/75 backdrop-blur-md"></div>

  <!-- Panel: double-bezel — outer shell + inner core -->
  <div class="relative w-full max-w-lg mx-4 rounded-[1.75rem] bg-white/[0.04] p-1.5 ring-1 ring-white/10 shadow-[0_30px_80px_-20px_rgba(0,0,0,0.8)] animate-fade-up">
    <div class="rounded-[calc(1.75rem-0.375rem)] bg-[var(--color-surface)] border border-[var(--color-border)] overflow-hidden">
      <div class="flex items-center justify-between px-5 py-4 border-b border-[var(--color-border)]">
        <h2 class="text-base font-semibold text-[var(--color-text)] tracking-tight">{title}</h2>
        <button
          onclick={close}
          class="text-[var(--color-muted)] hover:text-[var(--color-text)] transition-colors duration-200 rounded-lg p-1 hover:bg-white/5"
          aria-label="Close"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6L6 18M6 6l12 12" stroke-linecap="round"/></svg>
        </button>
      </div>
      <div class="px-5 py-4 max-h-[70vh] overflow-y-auto">
        {@render children?.()}
      </div>
      {#if footer}
      <div class="px-5 py-4 border-t border-[var(--color-border)] flex justify-end gap-2">
        {@render footer?.()}
      </div>
      {/if}
    </div>
  </div>
</div>
{/if}
