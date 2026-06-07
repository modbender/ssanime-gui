<script lang="ts">
  import { api, type Settings as SettingsType, type Profile } from '$lib/api'
  import Button from '$lib/components/Button.svelte'
  import Input from '$lib/components/Input.svelte'
  import Spinner from '$lib/components/Spinner.svelte'

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
  })

  async function load() {
    loading = true
    error = ''
    try {
      ;[settings, profiles] = await Promise.all([api.getSettings(), api.listProfiles()])
      form = { ...settings! }
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
      const updated = await api.putSettings(form)
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

  const backendOptions = [
    { value: null, label: 'Auto (embedded torrent)' },
    { value: 0, label: 'Embedded anacrolix/torrent' },
    { value: 1, label: 'yt-dlp (streaming/HLS)' },
  ]

  // Helper to convert null↔'' for optional text fields in the form
  function nullText(val: string | null): string { return val ?? '' }
  function textNull(val: string): string | null { return val.trim() || null }
</script>

<div class="flex flex-col h-full">
  <!-- Header -->
  <div class="flex items-center justify-between px-6 py-4 border-b border-[#2a2a35]">
    <h1 class="text-lg font-semibold text-[#e8e8f0]">Settings</h1>
    <div class="flex items-center gap-3">
      {#if saved}
        <span class="text-xs text-green-400 flex items-center gap-1">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
            <path d="M20 6L9 17l-5-5" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
          Saved
        </span>
      {/if}
      {#if saveError}
        <span class="text-xs text-red-400">{saveError}</span>
      {/if}
      <Button onclick={save} disabled={saving || loading}>
        {#if saving}<Spinner size={14} />{/if}
        Save settings
      </Button>
    </div>
  </div>

  <div class="flex-1 overflow-y-auto px-6 py-6">
    {#if loading}
      <div class="flex items-center justify-center h-64">
        <Spinner size={28} />
      </div>
    {:else if error}
      <div class="flex items-center justify-center h-64 text-red-400 text-sm">{error}</div>
    {:else}
      <div class="max-w-2xl space-y-8">

        <!-- Paths -->
        <section>
          <h2 class="text-xs font-semibold text-[#6b6b80] uppercase tracking-wider mb-4">Paths</h2>
          <div class="space-y-4">
            <div>
              <label class="block text-sm text-[#e8e8f0] mb-1.5">Download root</label>
              <Input bind:value={form.download_root} placeholder="/mnt/downloads" class="font-mono" />
              <p class="text-xs text-[#6b6b80] mt-1">Where source files are saved during download.</p>
            </div>
            <div>
              <label class="block text-sm text-[#e8e8f0] mb-1.5">Encoded root</label>
              <Input bind:value={form.encoded_root} placeholder="/mnt/archive" class="font-mono" />
              <p class="text-xs text-[#6b6b80] mt-1">Where finished encoded files are archived.</p>
            </div>
            <div>
              <label class="block text-sm text-[#e8e8f0] mb-1.5">Naming template</label>
              <Input bind:value={form.naming_template} placeholder={'${title}/S${season:02}E${ep:02}'} class="font-mono" />
            </div>
          </div>
        </section>

        <!-- Cleanup -->
        <section>
          <h2 class="text-xs font-semibold text-[#6b6b80] uppercase tracking-wider mb-4">Post-encode cleanup</h2>
          <div class="space-y-4">
            <div>
              <label class="block text-sm text-[#e8e8f0] mb-1.5">Cleanup policy</label>
              <select
                bind:value={form.cleanup_policy}
                class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] focus:outline-none focus:border-[#7c6af0] cursor-pointer"
              >
                {#each cleanupPolicies as p}
                  <option value={p.value}>{p.label}</option>
                {/each}
              </select>
            </div>
            {#if form.cleanup_policy === 'move'}
              <div>
                <label class="block text-sm text-[#e8e8f0] mb-1.5">Processed dir</label>
                <input
                  type="text"
                  value={nullText(form.processed_dir)}
                  oninput={(e) => { form.processed_dir = textNull((e.target as HTMLInputElement).value) }}
                  placeholder="/mnt/processed"
                  class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] placeholder:text-[#6b6b80] focus:outline-none focus:border-[#7c6af0] font-mono"
                />
              </div>
            {/if}
          </div>
        </section>

        <!-- Concurrency -->
        <section>
          <h2 class="text-xs font-semibold text-[#6b6b80] uppercase tracking-wider mb-4">Concurrency</h2>
          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="block text-sm text-[#e8e8f0] mb-1.5">Max simultaneous downloads</label>
              <input
                type="number"
                bind:value={form.concurrency_download}
                min="1"
                max="10"
                step="1"
                class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] focus:outline-none focus:border-[#7c6af0]"
              />
            </div>
            <div>
              <label class="block text-sm text-[#e8e8f0] mb-1.5">Max simultaneous encodes</label>
              <input
                type="number"
                bind:value={form.concurrency_encode}
                min="1"
                max="8"
                step="1"
                class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] focus:outline-none focus:border-[#7c6af0]"
              />
            </div>
          </div>
        </section>

        <!-- Default profile -->
        <section>
          <h2 class="text-xs font-semibold text-[#6b6b80] uppercase tracking-wider mb-4">Encode defaults</h2>
          <div>
            <label class="block text-sm text-[#e8e8f0] mb-1.5">Default encode profile</label>
            <select
              bind:value={form.default_profile_id}
              class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] focus:outline-none focus:border-[#7c6af0] cursor-pointer"
            >
              <option value={null}>— none —</option>
              {#each profiles as p (p.id)}
                <option value={p.id}>{p.name}{p.is_builtin ? ' (built-in)' : ''}</option>
              {/each}
            </select>
          </div>
        </section>

        <!-- Network -->
        <section>
          <h2 class="text-xs font-semibold text-[#6b6b80] uppercase tracking-wider mb-4">Network</h2>
          <div class="space-y-4">
            <div>
              <label class="block text-sm text-[#e8e8f0] mb-1.5">HTTP port</label>
              <input
                type="number"
                bind:value={form.port}
                min="1024"
                max="65535"
                step="1"
                class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] focus:outline-none focus:border-[#7c6af0]"
              />
              <p class="text-xs text-[#6b6b80] mt-1">Takes effect after restart.</p>
            </div>
            <label class="flex items-center gap-3 cursor-pointer">
              <div class="relative">
                <input type="checkbox" bind:checked={form.doh_enabled} class="sr-only" />
                <div class="w-10 h-5 rounded-full transition-colors {form.doh_enabled ? 'bg-[#7c6af0]' : 'bg-[#2a2a35]'}"></div>
                <div class="absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white transition-transform {form.doh_enabled ? 'translate-x-5' : 'translate-x-0'}"></div>
              </div>
              <div>
                <span class="text-sm text-[#e8e8f0]">DNS-over-HTTPS (DoH)</span>
                <p class="text-xs text-[#6b6b80]">Routes DNS via Cloudflare 1.1.1.1 — bypasses ISP DNS blocks (e.g. nyaa.si).</p>
              </div>
            </label>
          </div>
        </section>

        <!-- Download backend -->
        <section>
          <h2 class="text-xs font-semibold text-[#6b6b80] uppercase tracking-wider mb-4">Download backend</h2>
          <div>
            <select
              bind:value={form.download_backend}
              class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] focus:outline-none focus:border-[#7c6af0] cursor-pointer"
            >
              {#each backendOptions as opt}
                <option value={opt.value}>{opt.label}</option>
              {/each}
            </select>
          </div>
        </section>

        <!-- Binary paths -->
        <section>
          <h2 class="text-xs font-semibold text-[#6b6b80] uppercase tracking-wider mb-4">Binary paths</h2>
          <p class="text-xs text-[#6b6b80] mb-4">Leave empty to use auto-managed binaries in the app data directory.</p>
          <div class="space-y-4">
            <div>
              <label class="block text-sm text-[#e8e8f0] mb-1.5">ffmpeg path</label>
              <input
                type="text"
                value={nullText(form.ffmpeg_path)}
                oninput={(e) => { form.ffmpeg_path = textNull((e.target as HTMLInputElement).value) }}
                placeholder="auto (managed)"
                class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] placeholder:text-[#6b6b80] focus:outline-none focus:border-[#7c6af0] font-mono"
              />
            </div>
            <div>
              <label class="block text-sm text-[#e8e8f0] mb-1.5">yt-dlp path</label>
              <input
                type="text"
                value={nullText(form.ytdlp_path)}
                oninput={(e) => { form.ytdlp_path = textNull((e.target as HTMLInputElement).value) }}
                placeholder="auto (managed)"
                class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] placeholder:text-[#6b6b80] focus:outline-none focus:border-[#7c6af0] font-mono"
              />
            </div>
          </div>
        </section>

      </div>
    {/if}
  </div>
</div>
