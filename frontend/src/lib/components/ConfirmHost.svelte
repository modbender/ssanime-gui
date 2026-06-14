<script lang="ts">
  import Modal from '$lib/components/Modal.svelte'
  import Button from '$lib/components/Button.svelte'
  import { confirmState, resolveConfirm } from '$lib/confirm.svelte'

  // Modal closes itself on backdrop/escape via its bound `open`; mirror that into
  // a cancel so the pending promise resolves false.
  let open = $state(false)
  $effect(() => { open = confirmState.open })
  $effect(() => {
    if (!open && confirmState.open) resolveConfirm(false)
  })
</script>

<Modal bind:open title={confirmState.title}>
  {#snippet footer()}
    <Button variant="ghost" onclick={() => resolveConfirm(false)}>{confirmState.cancelLabel}</Button>
    <Button
      variant={confirmState.destructive ? 'destructive' : 'default'}
      onclick={() => resolveConfirm(true)}
    >
      {confirmState.confirmLabel}
    </Button>
  {/snippet}

  <p class="text-sm text-[var(--color-text-dim)]">{confirmState.message}</p>
</Modal>
