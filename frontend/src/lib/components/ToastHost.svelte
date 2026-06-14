<script lang="ts">
  import { toastState, dismissToast, type ToastKind } from '$lib/toast.svelte'

  // Per-kind accent + glyph. Colors come from the existing semantic CSS vars.
  const accent: Record<ToastKind, string> = {
    error: 'var(--color-error)',
    success: 'var(--color-success)',
    info: 'var(--accent)',
  }
</script>

<div
  class="pointer-events-none fixed bottom-4 right-4 z-[60] flex w-full max-w-sm flex-col gap-2"
  role="status"
  aria-live="polite"
>
  {#each toastState.toasts as t (t.id)}
    <div
      class="pointer-events-auto flex items-start gap-3 border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 shadow-[0_20px_50px_-20px_rgba(0,0,0,0.8)] animate-fade-up"
      style="border-left: 2px solid {accent[t.kind]};"
    >
      <span class="mt-0.5 shrink-0" style="color: {accent[t.kind]};" aria-hidden="true">
        {#if t.kind === 'error'}
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M12 8v5M12 16h.01" stroke-linecap="round"/></svg>
        {:else if t.kind === 'success'}
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25"><circle cx="12" cy="12" r="10"/><path d="m8 12 3 3 5-6" stroke-linecap="round" stroke-linejoin="round"/></svg>
        {:else}
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M12 11v5M12 8h.01" stroke-linecap="round"/></svg>
        {/if}
      </span>
      <span class="flex-1 break-words text-sm text-[var(--color-text)]">{t.message}</span>
      <button
        onclick={() => dismissToast(t.id)}
        class="shrink-0 text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
        aria-label="Dismiss"
      >
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6 6 18M6 6l12 12" stroke-linecap="round"/></svg>
      </button>
    </div>
  {/each}
</div>
