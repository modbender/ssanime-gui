<script lang="ts">
  import { navigate } from 'svelte-routing'
  import Modal from '$lib/components/Modal.svelte'
  import Button from '$lib/components/Button.svelte'
  import { gateState, closeGate } from '$lib/sources.svelte'

  function goToExtensions() {
    closeGate()
    navigate('/extensions')
  }
</script>

<Modal bind:open={gateState.open} title="Add a source first">
  {#snippet footer()}
    <Button variant="ghost" onclick={closeGate}>Cancel</Button>
    <Button onclick={goToExtensions}>
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
        <path d="M5 12h14M13 6l6 6-6 6" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
      Go to Extensions
    </Button>
  {/snippet}

  <div class="flex gap-4">
    <div class="shrink-0 w-11 h-11 bg-white/[0.04] ring-1 ring-white/10 flex items-center justify-center text-[var(--accent-text)]">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" aria-hidden="true">
        <path d="M14 7h5a2 2 0 0 1 2 2v5M10 21H5a2 2 0 0 1-2-2v-5M7 3 3 7l4 4M17 21l4-4-4-4" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
    </div>
    <div class="space-y-2 text-sm leading-relaxed text-[var(--color-text-dim)]">
      <p>
        Downloading needs at least one enabled source. ssanime-gui ships with none — you install
        sources yourself as extensions.
      </p>
      <p class="text-[var(--color-muted)]">
        Browsing and discovery keep working without one. Add a repository on the Extensions page,
        then enable a source to start downloading.
      </p>
    </div>
  </div>
</Modal>
