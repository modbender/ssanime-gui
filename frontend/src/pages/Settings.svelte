<script lang="ts">
  import { api, type Settings as SettingsType, type Profile } from '$lib/api'
  import Button from '$lib/components/Button.svelte'
  import Input from '$lib/components/Input.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import LogStream from '$lib/components/LogStream.svelte'
  import { scrollScrim } from '$lib/scrollScrim'

  type TabId = 'general' | 'processing' | 'network' | 'binaries' | 'logs'
  const tabs: { id: TabId; label: string }[] = [
    { id: 'general', label: 'General' },
    { id: 'processing', label: 'Processing' },
    { id: 'network', label: 'Network' },
    { id: 'binaries', label: 'Binaries' },
    { id: 'logs', label: 'Logs' },
  ]
  let activeTab = $state<TabId>('general')

  let settings = $state<SettingsType | null>(null)
  let profiles = $state<Profile[]>([])
  let loading = $state(true)
  let saving = $state(false)
  let saved = $state(false)
  let error = $state('')
  let saveError = $state('')

  // Working copy bound to form
  let form = $state<SettingsType>({
    download_root: '',
    encoded_root: '',
    cleanup_policy: 'keep',
    processed_dir: null,
    naming_template: '',
    download_backend: null,
    default_profile_id: null,
    concurrency_download: 2,
    concurrency_encode: 1,
    ffmpeg_path: null,
    ytdlp_path: null,
    port: 4773,
    doh_enabled: true,
    setup_completed: false,
    show_nsfw: false,
    trusted_release_groups: [],
  })

  function addTrustedGroup() {
    form.trusted_release_groups = [...form.trusted_release_groups, '']
  }
  function removeTrustedGroup(i: number) {
    form.trusted_release_groups = form.trusted_release_groups.filter((_, idx) => idx !== i)
  }
  function setTrustedGroup(i: number, val: string) {
    const next = [...form.trusted_release_groups]
    next[i] = val
    form.trusted_release_groups = next
  }
  // Empty after trimming blanks → backend treats as "no trust filter".
  const trustedGroupsEmpty = $derived(
    form.trusted_release_groups.every((g) => g.trim() === ''),
  )

  async function load() {
    loading = true
    error = ''
    try {
      ;[settings, profiles] = await Promise.all([api.getSettings(), api.listProfiles()])
      form = { ...settings! }
      // Only "Auto" (null) is a real backend today — external torrent clients and
      // yt-dlp are deferred. Normalize any legacy pinned client id to Auto so the
      // control and a subsequent save agree. (Auto resolves to the embedded
      // torrent client server-side.) Replace with a real client list when those
      // backends ship.
      form.download_backend = null
    } catch (e: any) {
      error = e.message
    } finally {
      loading = false
    }
  }

  $effect(() => { load() })

  async function save() {
    saving = true
    saved = false
    saveError = ''
    try {
      const payload: SettingsType = {
        ...form,
        trusted_release_groups: form.trusted_release_groups
          .map((g) => g.trim())
          .filter((g) => g !== ''),
      }
      const updated = await api.putSettings(payload)
      form = { ...updated }
      settings = { ...updated }
      saved = true
      setTimeout(() => { saved = false }, 2500)
    } catch (e: any) {
      saveError = e.message
    } finally {
      saving = false
    }
  }

  const cleanupPolicies = [
    { value: 'keep', label: 'Keep source files' },
    { value: 'delete', label: 'Delete source after encode' },
    { value: 'move', label: 'Move source to processed dir' },
  ]

  // Helper to convert null↔'' for optional text fields in the form
  function nullText(val: string | null): string { return val ?? '' }
  function textNull(val: string): string | null { return val.trim() || null }
</script>

<div class="flex flex-col h-full overflow-hidden" use:scrollScrim>
  <!-- Page header -->
  <div class="sticky top-0 z-10 px-6 sm:px-10 pt-4 bg-transparent backdrop-blur-0 border-b border-transparent transition-[background-color,border-color,backdrop-filter] duration-300 [.scrolled_&]:bg-[var(--color-bg)]/85 [.scrolled_&]:backdrop-blur-md [.scrolled_&]:border-[var(--color-border)]">
    <div class="flex items-center justify-between">
      <h1 class="text-[15px] font-semibold tracking-tight">Settings</h1>
      {#if activeTab !== 'logs'}
        <div class="flex items-center gap-3">
          {#if saved}
            <span class="text-xs text-[var(--color-success)] flex items-center gap-1">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
                <path d="M20 6L9 17l-5-5" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
              Saved
            </span>
          {/if}
          {#if saveError}
            <span class="text-xs text-[var(--color-error)]">{saveError}</span>
          {/if}
          <Button onclick={save} disabled={saving || loading}>
            {#if saving}<Spinner size={14} />{/if}
            Save settings
          </Button>
        </div>
      {/if}
    </div>

    <!-- Tab bar -->
    <div role="tablist" aria-label="Settings sections" class="flex items-center gap-1 -mb-px mt-3">
      {#each tabs as tab (tab.id)}
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === tab.id}
          onclick={() => { activeTab = tab.id }}
          class="px-3 py-2 text-sm font-medium border-b-2 transition-colors {activeTab === tab.id
            ? 'border-[var(--accent)] text-[var(--color-text)]'
            : 'border-transparent text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
        >
          {tab.label}
        </button>
      {/each}
    </div>
  </div>

  {#if activeTab === 'logs'}
    <LogStream />
  {:else}
    <div class="flex-1 overflow-y-auto px-6 sm:px-10 py-8 animate-fade-up">
      {#if loading}
        <div class="flex items-center justify-center h-64 text-[var(--color-muted)]">
          <Spinner size={28} />
        </div>
      {:else if error}
        <div class="flex items-center justify-center h-64 text-[var(--color-error)] text-sm">{error}</div>
      {:else}
        <div class="max-w-2xl space-y-6">

          {#if activeTab === 'general'}
            <!-- Paths -->
            <section class="border border-[var(--color-border)] bg-[var(--color-surface)] overflow-hidden">
              <div class="px-5 py-3 border-b border-[var(--color-border)] bg-[var(--color-surface-2)]">
                <h2 class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Paths</h2>
              </div>
              <div class="px-5 py-5 space-y-4">
                <div>
                  <label for="s-download-root" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">Download root</label>
                  <input id="s-download-root" type="text" bind:value={form.download_root} placeholder="/mnt/downloads" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] font-mono transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
                  <p class="text-xs text-[var(--color-muted)] mt-1">Where source files are saved during download.</p>
                </div>
                <div>
                  <label for="s-encoded-root" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">Encoded root</label>
                  <input id="s-encoded-root" type="text" bind:value={form.encoded_root} placeholder="/mnt/archive" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] font-mono transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
                  <p class="text-xs text-[var(--color-muted)] mt-1">Where finished encoded files are archived.</p>
                </div>
                <div>
                  <label for="s-naming" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">Naming template</label>
                  <input id="s-naming" type="text" bind:value={form.naming_template} placeholder={'${title}/S${season:02}E${ep:02}'} class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] font-mono transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
                </div>
              </div>
            </section>
          {/if}

          {#if activeTab === 'processing'}
            <!-- Concurrency -->
            <section class="border border-[var(--color-border)] bg-[var(--color-surface)] overflow-hidden">
              <div class="px-5 py-3 border-b border-[var(--color-border)] bg-[var(--color-surface-2)]">
                <h2 class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Concurrency</h2>
              </div>
              <div class="px-5 py-5">
                <div class="grid grid-cols-2 gap-4">
                  <div>
                    <label for="s-conc-dl" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">Max simultaneous downloads</label>
                    <input
                      id="s-conc-dl"
                      type="number"
                      bind:value={form.concurrency_download}
                      min="1"
                      max="10"
                      step="1"
                      class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors"
                    />
                  </div>
                  <div>
                    <label for="s-conc-enc" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">Max simultaneous encodes</label>
                    <input
                      id="s-conc-enc"
                      type="number"
                      bind:value={form.concurrency_encode}
                      min="1"
                      max="8"
                      step="1"
                      class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors"
                    />
                  </div>
                </div>
              </div>
            </section>

            <!-- Encode defaults -->
            <section class="border border-[var(--color-border)] bg-[var(--color-surface)] overflow-hidden">
              <div class="px-5 py-3 border-b border-[var(--color-border)] bg-[var(--color-surface-2)]">
                <h2 class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Encode defaults</h2>
              </div>
              <div class="px-5 py-5">
                <label for="s-default-profile" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">Default encode profile</label>
                <select
                  id="s-default-profile"
                  bind:value={form.default_profile_id}
                  class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors cursor-pointer"
                >
                  <option value={null}>— none —</option>
                  {#each profiles as p (p.id)}
                    <option value={p.id}>{p.name}{p.is_builtin ? ' (built-in)' : ''}</option>
                  {/each}
                </select>
              </div>
            </section>

            <!-- Trusted release groups -->
            <section class="border border-[var(--color-border)] bg-[var(--color-surface)] overflow-hidden">
              <div class="px-5 py-3 border-b border-[var(--color-border)] bg-[var(--color-surface-2)]">
                <h2 class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Trusted release groups</h2>
              </div>
              <div class="px-5 py-5 space-y-4">
                <p class="text-xs text-[var(--color-muted)]">
                  Source selection prefers these groups, in order — the first listed is preferred. Because the app
                  re-encodes everything itself, it wants clean original-subbed releases (e.g. SubsPlease, Erai-raws),
                  not re-encodes from secondary groups.
                </p>

                <div class="space-y-2">
                  {#each form.trusted_release_groups as group, i (i)}
                    <div class="flex items-center gap-2">
                      <span class="w-5 shrink-0 text-right text-xs font-medium tabular-nums text-[var(--color-muted)]">{i + 1}</span>
                      <input
                        type="text"
                        value={group}
                        oninput={(e) => setTrustedGroup(i, (e.target as HTMLInputElement).value)}
                        placeholder="e.g. SubsPlease"
                        class="h-9 flex-1 border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:outline-none focus:border-[var(--accent)] transition-colors"
                        aria-label={`Trusted release group ${i + 1}`}
                      />
                      <button
                        type="button"
                        onclick={() => removeTrustedGroup(i)}
                        class="flex h-9 w-9 shrink-0 items-center justify-center border border-[var(--color-border)] bg-[var(--color-surface-2)] text-[var(--color-muted)] transition-colors hover:border-[var(--color-border-strong)] hover:text-[var(--color-text)]"
                        title="Remove group"
                        aria-label={`Remove trusted release group ${i + 1}`}
                      >
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12" stroke-linecap="round" stroke-linejoin="round"/></svg>
                      </button>
                    </div>
                  {/each}
                </div>

                <Button variant="outline" size="sm" onclick={addTrustedGroup}>
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25" aria-hidden="true"><path d="M12 5v14M5 12h14" stroke-linecap="round" stroke-linejoin="round"/></svg>
                  Add group
                </Button>

                {#if trustedGroupsEmpty}
                  <p class="flex items-start gap-2 text-xs text-[var(--color-warning)]">
                    <svg class="mt-0.5 shrink-0" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" stroke-linecap="round" stroke-linejoin="round"/><path d="M12 9v4M12 17h.01" stroke-linecap="round" stroke-linejoin="round"/></svg>
                    <span>No trusted groups set — source selection won't filter by group and may download lower-quality re-encodes.</span>
                  </p>
                {/if}
              </div>
            </section>

            <!-- Download backend -->
            <section class="border border-[var(--color-border)] bg-[var(--color-surface)] overflow-hidden">
              <div class="px-5 py-3 border-b border-[var(--color-border)] bg-[var(--color-surface-2)]">
                <h2 class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Download backend</h2>
              </div>
              <div class="px-5 py-5">
                <label for="s-backend" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">Backend</label>
                <select
                  id="s-backend"
                  bind:value={form.download_backend}
                  class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors cursor-pointer"
                >
                  <option value={null}>Auto (embedded torrent)</option>
                </select>
                <p class="text-xs text-[var(--color-muted)] mt-1">Episodes download through the built-in torrent client. External clients and yt-dlp aren't available yet.</p>
              </div>
            </section>

            <!-- Post-encode cleanup -->
            <section class="border border-[var(--color-border)] bg-[var(--color-surface)] overflow-hidden">
              <div class="px-5 py-3 border-b border-[var(--color-border)] bg-[var(--color-surface-2)]">
                <h2 class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Post-encode cleanup</h2>
              </div>
              <div class="px-5 py-5 space-y-4">
                <div>
                  <label for="s-cleanup" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">Cleanup policy</label>
                  <select
                    id="s-cleanup"
                    bind:value={form.cleanup_policy}
                    class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors cursor-pointer"
                  >
                    {#each cleanupPolicies as p}
                      <option value={p.value}>{p.label}</option>
                    {/each}
                  </select>
                </div>
                {#if form.cleanup_policy === 'move'}
                  <div>
                    <label for="s-processed-dir" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">Processed dir</label>
                    <input
                      id="s-processed-dir"
                      type="text"
                      value={nullText(form.processed_dir)}
                      oninput={(e) => { form.processed_dir = textNull((e.target as HTMLInputElement).value) }}
                      placeholder="/mnt/processed"
                      class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:outline-none focus:border-[var(--accent)] transition-colors font-mono"
                    />
                  </div>
                {/if}
              </div>
            </section>
          {/if}

          {#if activeTab === 'network'}
            <!-- Network -->
            <section class="border border-[var(--color-border)] bg-[var(--color-surface)] overflow-hidden">
              <div class="px-5 py-3 border-b border-[var(--color-border)] bg-[var(--color-surface-2)]">
                <h2 class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Network</h2>
              </div>
              <div class="px-5 py-5 space-y-5">
                <div>
                  <label for="s-port" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">HTTP port</label>
                  <input
                    id="s-port"
                    type="number"
                    bind:value={form.port}
                    min="1024"
                    max="65535"
                    step="1"
                    class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors"
                  />
                  <p class="text-xs text-[var(--color-muted)] mt-1">Takes effect after restart.</p>
                </div>
                <label class="flex items-center gap-3 cursor-pointer">
                  <div class="relative shrink-0">
                    <input type="checkbox" bind:checked={form.doh_enabled} class="sr-only" aria-label="Enable DNS-over-HTTPS" />
                    <div class="w-10 h-5 transition-colors duration-200 {form.doh_enabled ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-border-strong)]'}"></div>
                    <div class="absolute top-0.5 left-0.5 w-4 h-4 bg-white transition-transform duration-200 {form.doh_enabled ? 'translate-x-5' : 'translate-x-0'}"></div>
                  </div>
                  <div>
                    <span class="text-sm font-medium text-[var(--color-text)]">DNS-over-HTTPS (DoH)</span>
                    <p class="text-xs text-[var(--color-muted)]">Routes DNS via Cloudflare 1.1.1.1 — bypasses ISP DNS blocks (e.g. nyaa.si).</p>
                  </div>
                </label>
              </div>
            </section>
          {/if}

          {#if activeTab === 'binaries'}
            <!-- Binary paths -->
            <section class="border border-[var(--color-border)] bg-[var(--color-surface)] overflow-hidden">
              <div class="px-5 py-3 border-b border-[var(--color-border)] bg-[var(--color-surface-2)]">
                <h2 class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Binary paths</h2>
              </div>
              <div class="px-5 py-5 space-y-4">
                <p class="text-xs text-[var(--color-muted)]">Leave empty to use auto-managed binaries in the app data directory.</p>
                <div>
                  <label for="s-ffmpeg" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">ffmpeg path</label>
                  <input
                    id="s-ffmpeg"
                    type="text"
                    value={nullText(form.ffmpeg_path)}
                    oninput={(e) => { form.ffmpeg_path = textNull((e.target as HTMLInputElement).value) }}
                    placeholder="auto (managed)"
                    class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:outline-none focus:border-[var(--accent)] transition-colors font-mono"
                  />
                </div>
                <div>
                  <label for="s-ytdlp" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">yt-dlp path</label>
                  <input
                    id="s-ytdlp"
                    type="text"
                    value={nullText(form.ytdlp_path)}
                    oninput={(e) => { form.ytdlp_path = textNull((e.target as HTMLInputElement).value) }}
                    placeholder="auto (managed)"
                    class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:outline-none focus:border-[var(--accent)] transition-colors font-mono"
                  />
                </div>
              </div>
            </section>
          {/if}

        </div>
      {/if}
    </div>
  {/if}
</div>
