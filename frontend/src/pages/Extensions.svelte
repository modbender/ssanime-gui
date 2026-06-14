<script lang="ts">
  import { api, type ExtensionRepo, type Extension, type Settings as SettingsType } from '$lib/api'
  import Button from '$lib/components/Button.svelte'
  import Modal from '$lib/components/Modal.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import { reloadSources } from '$lib/sources.svelte'
  import { formatDate } from '$lib/utils'

  let repos = $state<ExtensionRepo[]>([])
  let extensions = $state<Extension[]>([])
  let settings = $state<SettingsType | null>(null)
  let loading = $state(true)
  let error = $state('')

  // Add-repo modal
  let addOpen = $state(false)
  let addName = $state('')
  let addUrl = $state('')
  let adding = $state(false)

  // Per-row in-flight markers
  let syncing = $state<number | null>(null)
  let removingRepo = $state<number | null>(null)
  let togglingExt = $state<number | null>(null)
  let removingExt = $state<number | null>(null)
  let savingNsfw = $state(false)

  // Extension ids whose icon URL failed to load — fall back to the placeholder
  // glyph (a dead/blocked icon URL otherwise renders the browser's broken-image
  // icon). Keyed by id so one bad icon doesn't affect the others.
  let iconFailed = $state<Record<number, boolean>>({})

  // Transient page-level error banner (replaces native alert(), which renders as
  // a "127.0.0.1 says…" box and is unreliable inside the desktop webview).
  let actionError = $state('')

  // In-app confirm dialog (replaces native confirm(), same webview reasons).
  let confirmOpen = $state(false)
  let confirmTitle = $state('')
  let confirmMessage = $state('')
  let confirmLabel = $state('Remove')
  let confirmBusy = $state(false)
  let confirmAction: (() => Promise<void>) | null = null

  function askConfirm(title: string, message: string, action: () => Promise<void>, label = 'Remove') {
    confirmTitle = title
    confirmMessage = message
    confirmLabel = label
    confirmAction = action
    confirmOpen = true
  }

  async function runConfirm() {
    if (!confirmAction) return
    confirmBusy = true
    try {
      await confirmAction()
      confirmOpen = false
    } finally {
      confirmBusy = false
    }
  }

  const showNsfw = $derived(settings?.show_nsfw ?? false)
  const visibleExtensions = $derived(
    showNsfw ? extensions : extensions.filter((e) => !e.nsfw),
  )

  async function load() {
    loading = true
    error = ''
    try {
      ;[repos, extensions, settings] = await Promise.all([
        api.listExtensionRepos(),
        api.listExtensions(),
        api.getSettings(),
      ])
    } catch (e: any) {
      error = e.message
    } finally {
      loading = false
    }
  }

  $effect(() => { load() })

  // Keep the global sources signal and the local list in lockstep.
  async function refreshExtensions() {
    extensions = await api.listExtensions()
    await reloadSources()
  }

  function openAdd() {
    addName = ''
    addUrl = ''
    addOpen = true
  }

  async function addRepo() {
    if (!addName.trim() || !addUrl.trim()) return
    adding = true
    try {
      await api.createExtensionRepo({ name: addName.trim(), url: addUrl.trim() })
      addOpen = false
      repos = await api.listExtensionRepos()
    } catch (e: any) {
      actionError = e.message
    } finally {
      adding = false
    }
  }

  async function syncRepo(repo: ExtensionRepo) {
    if (syncing != null) return
    syncing = repo.id
    try {
      await api.syncExtensionRepo(repo.id)
      ;[repos] = await Promise.all([api.listExtensionRepos()])
      await refreshExtensions()
    } catch (e: any) {
      actionError = e.message
    } finally {
      syncing = null
    }
  }

  function removeRepo(repo: ExtensionRepo) {
    const owned = extensions.filter((e) => e.repo_id === repo.id).length
    const message = owned > 0
      ? `This also uninstalls ${owned} installed source${owned === 1 ? '' : 's'} from this repository.`
      : 'This repository has no installed sources.'
    askConfirm(`Remove repository "${repo.name}"?`, message, async () => {
      removingRepo = repo.id
      try {
        await api.deleteExtensionRepo(repo.id)
        repos = repos.filter((r) => r.id !== repo.id)
        // The backend cascade-deletes the repo's sources; reflect that here so a
        // phantom row doesn't linger in the installed list.
        await refreshExtensions()
      } catch (e: any) {
        actionError = e.message
      } finally {
        removingRepo = null
      }
    })
  }

  async function toggleExtension(ext: Extension) {
    if (togglingExt != null) return
    togglingExt = ext.id
    try {
      const updated = ext.enabled
        ? await api.disableExtension(ext.id)
        : await api.enableExtension(ext.id)
      extensions = extensions.map((e) => (e.id === ext.id ? updated : e))
      await reloadSources()
    } catch (e: any) {
      actionError = e.message
    } finally {
      togglingExt = null
    }
  }

  function removeExtension(ext: Extension) {
    askConfirm(`Remove source "${ext.name}"?`, 'This source will be uninstalled and unregistered.', async () => {
      removingExt = ext.id
      try {
        await api.uninstallExtension(ext.id)
        extensions = extensions.filter((e) => e.id !== ext.id)
        await reloadSources()
      } catch (e: any) {
        // The row may already be gone server-side (e.g. removed with its repo).
        // Reconcile against the server before surfacing an error.
        const fresh = await api.listExtensions().catch(() => null)
        if (fresh && !fresh.some((x) => x.id === ext.id)) {
          extensions = fresh
          await reloadSources()
        } else {
          actionError = e.message
        }
      } finally {
        removingExt = null
      }
    })
  }

  async function toggleNsfw() {
    if (!settings || savingNsfw) return
    savingNsfw = true
    const next = { ...settings, show_nsfw: !settings.show_nsfw }
    try {
      settings = await api.putSettings(next)
    } catch (e: any) {
      actionError = e.message
    } finally {
      savingNsfw = false
    }
  }
</script>

<div class="flex flex-col h-full overflow-y-auto">
  <!-- Page header -->
  <div class="sticky top-0 z-10 flex items-center justify-between px-6 sm:px-10 py-4 border-b border-[var(--color-border)] bg-[var(--color-bg)]/95 backdrop-blur-md">
    <h1 class="text-[15px] font-semibold tracking-tight">Extensions</h1>
    <Button onclick={openAdd}>
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
        <path d="M12 5v14M5 12h14" stroke-linecap="round"/>
      </svg>
      Add repository
    </Button>
  </div>

  <div class="flex-1 px-6 sm:px-10 py-8 animate-fade-up">
    {#if actionError}
      <div class="mb-6 flex items-start gap-3 border border-[var(--color-error)]/30 bg-[var(--color-error)]/10 px-4 py-3 text-sm text-[var(--color-error)]">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="mt-0.5 shrink-0" aria-hidden="true">
          <circle cx="12" cy="12" r="10"/><path d="M12 8v5M12 16h.01" stroke-linecap="round"/>
        </svg>
        <span class="flex-1 break-words">{actionError}</span>
        <button
          onclick={() => { actionError = '' }}
          class="shrink-0 text-[var(--color-error)]/70 hover:text-[var(--color-error)] transition-colors"
          aria-label="Dismiss error"
        >
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6 6 18M6 6l12 12" stroke-linecap="round"/></svg>
        </button>
      </div>
    {/if}
    {#if loading}
      <div class="flex items-center justify-center h-64 text-[var(--color-muted)]">
        <Spinner size={28} />
      </div>
    {:else if error}
      <div class="flex items-center justify-center h-64 text-[var(--color-error)] text-sm">{error}</div>
    {:else}
      <div class="max-w-3xl space-y-8">

        <!-- Repositories -->
        <section class="border border-[var(--color-border)] bg-[var(--color-surface)] overflow-hidden">
          <div class="px-5 py-3 border-b border-[var(--color-border)] bg-[var(--color-surface-2)] flex items-center justify-between">
            <h2 class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Repositories</h2>
            {#if repos.length > 0}
              <span class="text-xs font-medium tabular-nums text-[var(--color-muted)]">{repos.length}</span>
            {/if}
          </div>

          {#if repos.length === 0}
            <div class="px-5 py-10 text-center space-y-2">
              <p class="text-sm font-medium text-[var(--color-text)]">No repositories yet</p>
              <p class="text-sm text-[var(--color-muted)] max-w-md mx-auto">
                Paste a repository index URL to fetch installable sources. SSAnime doesn't bundle
                or suggest any — add one you trust.
              </p>
              <div class="pt-2">
                <Button onclick={openAdd}>Add repository</Button>
              </div>
            </div>
          {:else}
            <ul class="divide-y divide-[var(--color-border)]/60">
              {#each repos as repo (repo.id)}
                <li class="flex items-center gap-4 px-5 py-4 hover:bg-white/[0.02] transition-colors duration-200">
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2 mb-1 flex-wrap">
                      <span class="text-sm font-medium text-[var(--color-text)]">{repo.name}</span>
                      {#if !repo.enabled}
                        <span class="inline-flex items-center bg-[var(--color-surface-2)] px-2.5 py-0.5 text-[11px] font-medium text-[var(--color-muted)] ring-1 ring-[var(--color-border)]">Disabled</span>
                      {/if}
                    </div>
                    <div class="flex items-center gap-3 text-xs text-[var(--color-muted)] flex-wrap">
                      <span class="font-mono truncate max-w-md text-[var(--color-text-dim)]" title={repo.url}>{repo.url}</span>
                      {#if repo.last_synced_at}
                        <span>Synced {formatDate(repo.last_synced_at)}</span>
                      {:else}
                        <span class="text-[var(--color-warning)]">Never synced</span>
                      {/if}
                    </div>
                  </div>

                  <div class="flex items-center gap-1 shrink-0">
                    <Button
                      variant="secondary"
                      size="sm"
                      onclick={() => syncRepo(repo)}
                      disabled={syncing === repo.id}
                      title="Re-fetch this repository's index and install/update its sources"
                    >
                      {#if syncing === repo.id}<Spinner size={12} />{:else}
                        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25" aria-hidden="true">
                          <path d="M23 4v6h-6M1 20v-6h6" stroke-linecap="round" stroke-linejoin="round"/>
                          <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" stroke-linecap="round" stroke-linejoin="round"/>
                        </svg>
                      {/if}
                      Sync
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onclick={() => removeRepo(repo)}
                      disabled={removingRepo === repo.id}
                      title="Remove repository"
                      class="hover:text-[var(--color-error)]"
                    >
                      {#if removingRepo === repo.id}<Spinner size={12} />{:else}
                        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
                          <polyline points="3 6 5 6 21 6" stroke-linecap="round" stroke-linejoin="round"/>
                          <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" stroke-linecap="round" stroke-linejoin="round"/>
                          <path d="M10 11v6M14 11v6M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2" stroke-linecap="round" stroke-linejoin="round"/>
                        </svg>
                      {/if}
                    </Button>
                  </div>
                </li>
              {/each}
            </ul>
          {/if}
        </section>

        <!-- Installed sources -->
        <section class="border border-[var(--color-border)] bg-[var(--color-surface)] overflow-hidden">
          <div class="px-5 py-3 border-b border-[var(--color-border)] bg-[var(--color-surface-2)] flex items-center justify-between gap-4">
            <div class="flex items-center gap-2.5">
              <h2 class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Installed sources</h2>
              {#if visibleExtensions.length > 0}
                <span class="text-xs font-medium tabular-nums text-[var(--color-muted)]">{visibleExtensions.length}</span>
              {/if}
            </div>
            <!-- Show NSFW toggle -->
            <label class="flex items-center gap-2 cursor-pointer">
              <span class="text-[11px] font-medium text-[var(--color-muted)]">Show NSFW sources</span>
              <button
                type="button"
                class="shrink-0 w-9 h-[18px] transition-colors duration-200 relative focus-visible:outline-2 focus-visible:outline-[var(--accent)] cursor-pointer disabled:opacity-50 {showNsfw ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-border-strong)]'}"
                onclick={toggleNsfw}
                role="switch"
                aria-checked={showNsfw}
                aria-label="Show NSFW sources"
                disabled={savingNsfw}
              >
                <span class="absolute top-0.5 left-0.5 w-3.5 h-3.5 bg-white transition-transform duration-200 {showNsfw ? 'translate-x-[18px]' : 'translate-x-0'}"></span>
              </button>
            </label>
          </div>

          {#if extensions.length === 0}
            <div class="px-5 py-10 text-center space-y-2">
              <p class="text-sm font-medium text-[var(--color-text)]">No sources installed</p>
              <p class="text-sm text-[var(--color-muted)] max-w-md mx-auto">
                Add a repository above and hit <span class="font-medium text-[var(--color-text-dim)]">Sync</span> to fetch
                its sources. Once a source is enabled you can download and track series.
              </p>
            </div>
          {:else if visibleExtensions.length === 0}
            <div class="px-5 py-10 text-center space-y-2">
              <p class="text-sm font-medium text-[var(--color-text)]">All sources hidden</p>
              <p class="text-sm text-[var(--color-muted)] max-w-md mx-auto">
                Every installed source is marked NSFW. Toggle <span class="font-medium text-[var(--color-text-dim)]">Show NSFW sources</span> to reveal them.
              </p>
            </div>
          {:else}
            <ul class="divide-y divide-[var(--color-border)]/60">
              {#each visibleExtensions as ext (ext.id)}
                <li class="flex items-center gap-4 px-5 py-4 hover:bg-white/[0.02] transition-colors duration-200">
                  <!-- Enable toggle -->
                  <button
                    class="shrink-0 w-10 h-5 transition-colors duration-200 relative focus-visible:outline-2 focus-visible:outline-[var(--accent)] cursor-pointer disabled:opacity-50 {ext.enabled ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-border-strong)]'}"
                    onclick={() => toggleExtension(ext)}
                    disabled={togglingExt === ext.id}
                    role="switch"
                    aria-checked={ext.enabled}
                    title={ext.enabled ? 'Disable source' : 'Enable source'}
                    aria-label={ext.enabled ? 'Disable source' : 'Enable source'}
                  >
                    <span class="absolute top-0.5 left-0.5 w-4 h-4 bg-white transition-transform duration-200 {ext.enabled ? 'translate-x-5' : 'translate-x-0'}"></span>
                  </button>

                  <!-- Icon -->
                  <div class="shrink-0 w-9 h-9 bg-[var(--color-surface-2)] ring-1 ring-[var(--color-border)] flex items-center justify-center overflow-hidden text-[var(--color-faint)]">
                    {#if ext.icon && !iconFailed[ext.id]}
                      <img
                        src={ext.icon}
                        alt=""
                        class="w-full h-full object-cover"
                        loading="lazy"
                        onerror={() => { iconFailed[ext.id] = true }}
                      />
                    {:else}
                      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" aria-hidden="true">
                        <path d="M14 7h5a2 2 0 0 1 2 2v5M10 21H5a2 2 0 0 1-2-2v-5M7 3 3 7l4 4M17 21l4-4-4-4" stroke-linecap="round" stroke-linejoin="round"/>
                      </svg>
                    {/if}
                  </div>

                  <!-- Info -->
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="text-sm font-medium text-[var(--color-text)] truncate">{ext.name}</span>
                      <span class="inline-flex items-center bg-white/[0.06] px-2 py-0.5 text-[11px] font-medium tabular-nums text-[var(--color-text-dim)] ring-1 ring-white/10">v{ext.version}</span>
                      {#if ext.lang}
                        <span class="inline-flex items-center bg-white/[0.06] px-2 py-0.5 text-[11px] font-medium uppercase text-[var(--color-text-dim)] ring-1 ring-white/10">{ext.lang}</span>
                      {/if}
                      {#if ext.nsfw}
                        <span class="inline-flex items-center bg-[var(--color-error)]/15 px-2 py-0.5 text-[11px] font-semibold text-[var(--color-error)] ring-1 ring-[var(--color-error)]/30">NSFW</span>
                      {/if}
                      {#if !ext.enabled}
                        <span class="inline-flex items-center bg-[var(--color-surface-2)] px-2 py-0.5 text-[11px] font-medium text-[var(--color-muted)] ring-1 ring-[var(--color-border)]">Disabled</span>
                      {/if}
                    </div>
                  </div>

                  <!-- Remove -->
                  <Button
                    variant="ghost"
                    size="icon"
                    onclick={() => removeExtension(ext)}
                    disabled={removingExt === ext.id}
                    title="Remove source"
                    class="shrink-0 hover:text-[var(--color-error)]"
                  >
                    {#if removingExt === ext.id}<Spinner size={12} />{:else}
                      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
                        <polyline points="3 6 5 6 21 6" stroke-linecap="round" stroke-linejoin="round"/>
                        <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" stroke-linecap="round" stroke-linejoin="round"/>
                        <path d="M10 11v6M14 11v6M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2" stroke-linecap="round" stroke-linejoin="round"/>
                      </svg>
                    {/if}
                  </Button>
                </li>
              {/each}
            </ul>
          {/if}
        </section>

      </div>
    {/if}
  </div>
</div>

<!-- Confirm dialog (in-app replacement for native confirm) -->
<Modal bind:open={confirmOpen} title={confirmTitle}>
  {#snippet footer()}
    <Button variant="ghost" onclick={() => { confirmOpen = false }} disabled={confirmBusy}>Cancel</Button>
    <Button variant="destructive" onclick={runConfirm} disabled={confirmBusy}>
      {#if confirmBusy}<Spinner size={14} />{/if}
      {confirmLabel}
    </Button>
  {/snippet}

  <p class="text-sm text-[var(--color-text-dim)]">{confirmMessage}</p>
</Modal>

<!-- Add repository modal -->
<Modal bind:open={addOpen} title="Add repository">
  {#snippet footer()}
    <Button variant="ghost" onclick={() => { addOpen = false }}>Cancel</Button>
    <Button onclick={addRepo} disabled={adding || !addName.trim() || !addUrl.trim()}>
      {#if adding}<Spinner size={14} />{/if}
      Add
    </Button>
  {/snippet}

  <div class="space-y-4">
    <div>
      <label for="repo-name" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Name</label>
      <input
        id="repo-name"
        type="text"
        bind:value={addName}
        placeholder="My sources"
        class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]"
      />
    </div>
    <div>
      <label for="repo-url" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Repository index URL</label>
      <input
        id="repo-url"
        type="text"
        bind:value={addUrl}
        placeholder="https://example.com/index.json"
        class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] font-mono transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]"
      />
      <p class="text-xs text-[var(--color-muted)] mt-1.5">
        Paste a source repository's index URL. Sources are unaffiliated with SSAnime — add only
        ones you trust.
      </p>
    </div>
  </div>
</Modal>
