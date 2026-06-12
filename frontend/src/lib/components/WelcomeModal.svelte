<script lang="ts">
  import { navigate } from 'svelte-routing'
  import { api } from '$lib/api'
  import Modal from '$lib/components/Modal.svelte'
  import Button from '$lib/components/Button.svelte'
  import { APP_NAME } from '$lib/app'

  let open = $state(false)

  $effect(() => {
    (async () => {
      try {
        const settings = await api.getSettings()
        if (!settings.setup_completed) {
          open = true
          // Persist optimistically so it never shows twice, even if the user
          // closes the tab without choosing an action.
          api.putSettings({ ...settings, setup_completed: true }).catch(() => {})
        }
      } catch {
        // Settings unreachable on first paint — stay silent, don't block browsing.
      }
    })()
  })

  function dismiss() {
    open = false
  }

  function goToExtensions() {
    open = false
    navigate('/extensions')
  }
</script>

<Modal bind:open title="Welcome to {APP_NAME}">
  {#snippet footer()}
    <Button variant="ghost" onclick={dismiss}>Dismiss</Button>
    <Button onclick={goToExtensions}>
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
        <path d="M5 12h14M13 6l6 6-6 6" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
      Go to Extensions
    </Button>
  {/snippet}

  <div class="space-y-4 text-sm leading-relaxed text-[var(--color-text-dim)]">
    <p>
      Browse trending and seasonal anime right away — discovery works out of the box.
    </p>
    <p>
      To actually <span class="font-medium text-[var(--color-text)]">download</span> anything, the
      app needs a <span class="font-medium text-[var(--color-text)]">source</span>. Sources aren't
      bundled — you add them yourself as extensions by pasting a repository URL on the Extensions
      page.
    </p>
    <p class="text-[var(--color-muted)]">
      You can do this now or later; download actions will remind you when a source is missing.
    </p>
  </div>
</Modal>
