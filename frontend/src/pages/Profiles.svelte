<script lang="ts">
  import { api, type Profile, type ResolvedProfile } from '$lib/api'
  import { COMMON_LANGUAGES, languageName } from '$lib/languages'
  import Badge from '$lib/components/Badge.svelte'
  import Button from '$lib/components/Button.svelte'
  import Input from '$lib/components/Input.svelte'
  import Modal from '$lib/components/Modal.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import { toast } from '$lib/toast.svelte'
  import { confirm } from '$lib/confirm.svelte'
  import { errMessage } from '$lib/utils'
  import { scrollScrim } from '$lib/scrollScrim'

  let profiles = $state<Profile[]>([])
  let loading = $state(true)
  let error = $state('')

  // Edit modal
  let editOpen = $state(false)
  let editMode = $state<'create' | 'edit'>('create')
  let saving = $state(false)
  let deleting = $state<number | null>(null)
  let editingId = $state<number | null>(null)
  let formError = $state('')

  // Resolved preview modal
  let resolvedOpen = $state(false)
  let resolved = $state<ResolvedProfile | null>(null)
  let resolvedName = $state('')
  let loadingResolved = $state(false)

  // Form — null means "inherit from parent"
  type ProfileForm = {
    name: string
    parent_id: string // stringified for select
    crf: string
    preset: string
    codec: string
    audio: string
    container: string
    scale: string
    x265_params: string
    deblock: string
    psy_rd: string
    psy_rdoq: string
    aq_strength: string
    aq_mode: string
    bit_depth: string // stringified for select, '' = inherit
    smartblur: boolean | null
    deinterlace: boolean | null
    deband: boolean | null
    // Language tracks — `wildcard` ⇒ serialize null (All for MKV / Default for MP4),
    // `specific` ⇒ serialize the codes array (MP4 holds a single element).
    audio_lang_mode: LangMode
    audio_langs: string[]
    subtitle_lang_mode: LangMode
    subtitle_langs: string[]
    output_resolutions: string // comma-separated
  }

  type LangMode = 'wildcard' | 'specific'

  let form = $state<ProfileForm>({
    name: '',
    parent_id: '',
    crf: '',
    preset: '',
    codec: '',
    audio: '',
    container: '',
    scale: '',
    x265_params: '',
    deblock: '',
    psy_rd: '',
    psy_rdoq: '',
    aq_strength: '',
    aq_mode: '',
    bit_depth: '',
    smartblur: null,
    deinterlace: null,
    deband: null,
    audio_lang_mode: 'wildcard',
    audio_langs: [],
    subtitle_lang_mode: 'wildcard',
    subtitle_langs: [],
    output_resolutions: '',
  })

  async function load() {
    loading = true
    error = ''
    try {
      profiles = await api.listProfiles()
    } catch (e: unknown) {
      error = errMessage(e)
    } finally {
      loading = false
    }
  }

  $effect(() => { load() })

  function emptyForm(): ProfileForm {
    return {
      name: '',
      parent_id: '',
      crf: '',
      preset: '',
      codec: '',
      audio: '',
      container: '',
      scale: '',
      x265_params: '',
      deblock: '',
      psy_rd: '',
      psy_rdoq: '',
      aq_strength: '',
      aq_mode: '',
      bit_depth: '',
      smartblur: null,
      deinterlace: null,
      deband: null,
      audio_lang_mode: 'wildcard',
      audio_langs: [],
      subtitle_lang_mode: 'wildcard',
      subtitle_langs: [],
      output_resolutions: '',
    }
  }

  function profileToForm(p: Profile): ProfileForm {
    return {
      name: p.name,
      parent_id: p.parent_id != null ? String(p.parent_id) : '',
      crf: p.crf != null ? String(p.crf) : '',
      preset: p.preset ?? '',
      codec: p.codec ?? '',
      audio: p.audio ?? '',
      container: p.container ?? '',
      scale: p.scale != null ? String(p.scale) : '',
      x265_params: p.x265_params ?? '',
      deblock: p.deblock ?? '',
      psy_rd: p.psy_rd != null ? String(p.psy_rd) : '',
      psy_rdoq: p.psy_rdoq != null ? String(p.psy_rdoq) : '',
      aq_strength: p.aq_strength != null ? String(p.aq_strength) : '',
      aq_mode: p.aq_mode != null ? String(p.aq_mode) : '',
      bit_depth: p.bit_depth != null ? String(p.bit_depth) : '',
      smartblur: p.smartblur ?? null,
      deinterlace: p.deinterlace ?? null,
      deband: p.deband ?? null,
      audio_lang_mode: p.audio_languages == null ? 'wildcard' : 'specific',
      audio_langs: p.audio_languages ?? [],
      subtitle_lang_mode: p.subtitle_languages == null ? 'wildcard' : 'specific',
      subtitle_langs: p.subtitle_languages ?? [],
      output_resolutions: p.output_resolutions?.join(', ') ?? '',
    }
  }

  // Serialize a language control to the 3-state wire value: wildcard ⇒ null,
  // specific ⇒ the codes array (MP4 ⇒ at most one, the single-track pick).
  function langField(mode: LangMode, langs: string[], container: string): string[] | null {
    if (mode !== 'specific') return null
    return container === 'mp4' ? langs.slice(0, 1) : langs
  }

  function formToBody(f: ProfileForm): Partial<Profile> & { name: string } {
    return {
      name: f.name,
      parent_id: f.parent_id ? Number(f.parent_id) : null,
      crf: f.crf ? Number(f.crf) : null,
      preset: f.preset || null,
      codec: f.codec || null,
      audio: f.audio || null,
      container: f.container || null,
      scale: f.scale ? Number(f.scale) : null,
      x265_params: f.x265_params || null,
      deblock: f.deblock || null,
      psy_rd: f.psy_rd ? Number(f.psy_rd) : null,
      psy_rdoq: f.psy_rdoq ? Number(f.psy_rdoq) : null,
      aq_strength: f.aq_strength ? Number(f.aq_strength) : null,
      aq_mode: f.aq_mode ? Number(f.aq_mode) : null,
      bit_depth: f.bit_depth ? Number(f.bit_depth) : null,
      smartblur: f.smartblur,
      deinterlace: f.deinterlace,
      deband: f.deband,
      // burn_subs is derived from container, not a free toggle: MP4 ⇒ true,
      // MKV ⇒ false, unset container ⇒ inherit (null).
      burn_subs: f.container === 'mp4' ? true : f.container === 'mkv' ? false : null,
      // Wildcard mode ⇒ null (All / Default); specific ⇒ the codes array. MP4 is a
      // single-track pick, so clamp specific selections to one code.
      audio_languages: langField(f.audio_lang_mode, f.audio_langs, f.container),
      subtitle_languages: langField(f.subtitle_lang_mode, f.subtitle_langs, f.container),
      output_resolutions: f.output_resolutions
        ? f.output_resolutions.split(',').map(s => Number(s.trim())).filter(Boolean)
        : null,
    }
  }

  function openCreate() {
    editMode = 'create'
    editingId = null
    form = emptyForm()
    formError = ''
    editOpen = true
  }

  function openEdit(p: Profile) {
    editMode = 'edit'
    editingId = p.id
    form = profileToForm(p)
    formError = ''
    editOpen = true
  }

  async function save() {
    if (!form.name.trim()) return
    saving = true
    formError = ''
    try {
      const body = formToBody(form)
      if (editMode === 'create') {
        await api.createProfile(body)
      } else if (editingId != null) {
        await api.patchProfile(editingId, body)
      }
      editOpen = false
      await load()
    } catch (e: unknown) {
      // Surface validation failures (e.g. 400 on an unknown language code) inline.
      formError = errMessage(e)
    } finally {
      saving = false
    }
  }

  async function deleteProfile(id: number) {
    if (!(await confirm({
      title: 'Delete profile?',
      message: 'This profile will be permanently removed.',
      confirmLabel: 'Delete',
      destructive: true,
    }))) return
    deleting = id
    try {
      await api.deleteProfile(id)
      profiles = profiles.filter(p => p.id !== id)
    } catch (e: unknown) {
      toast.error(errMessage(e))
    } finally {
      deleting = null
    }
  }

  async function showResolved(p: Profile) {
    resolvedName = p.name
    resolvedOpen = true
    resolved = null
    loadingResolved = true
    try {
      resolved = await api.getResolvedProfile(p.id)
    } catch (e: unknown) {
      toast.error(errMessage(e))
      resolvedOpen = false
    } finally {
      loadingResolved = false
    }
  }

  const parentOptions = $derived(profiles.filter(p => editingId == null || p.id !== editingId))

  function parentName(id: number | null) {
    if (!id) return null
    return profiles.find(p => p.id === id)?.name ?? `#${id}`
  }

  const presets = ['ultrafast', 'superfast', 'veryfast', 'faster', 'fast', 'medium', 'slow', 'slower', 'veryslow', 'placebo']

  const codecs = [
    { value: 'x265', label: 'x265 (CPU)' },
    { value: 'gpu-auto', label: 'GPU (auto)' },
  ]

  const isMp4 = $derived(form.container === 'mp4')

  // Toggle a code in a multi-select (MKV specific mode).
  function toggleLang(field: 'audio_langs' | 'subtitle_langs', code: string) {
    const cur = form[field]
    form[field] = cur.includes(code) ? cur.filter(c => c !== code) : [...cur, code]
  }
</script>

<div class="flex flex-col h-full overflow-y-auto" use:scrollScrim>
  <!-- Page header -->
  <div class="sticky top-0 z-10 flex items-center justify-between px-6 sm:px-10 py-4 bg-transparent backdrop-blur-0 border-b border-transparent transition-[background-color,border-color,backdrop-filter] duration-300 [.scrolled_&]:bg-[var(--color-bg)]/85 [.scrolled_&]:backdrop-blur-md [.scrolled_&]:border-[var(--color-border)]">
    <div class="flex items-baseline gap-2.5">
      <h1 class="text-[15px] font-semibold tracking-tight">Encode profiles</h1>
      {#if !loading && profiles.length > 0}
        <span class="text-xs font-medium tabular-nums text-[var(--color-muted)]">{profiles.length}</span>
      {/if}
    </div>
    <Button onclick={openCreate}>
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
        <path d="M12 5v14M5 12h14" stroke-linecap="round"/>
      </svg>
      New profile
    </Button>
  </div>

  <div class="flex-1 px-6 sm:px-10 py-8 animate-fade-up">
    {#if loading}
      <div class="flex items-center justify-center h-64 text-[var(--color-muted)]">
        <Spinner size={28} />
      </div>
    {:else if error}
      <div class="flex items-center justify-center h-64 text-[var(--color-error)] text-sm">{error}</div>
    {:else if profiles.length === 0}
      <!-- Empty state -->
      <div class="flex flex-col items-center justify-center gap-4 py-24 text-center">
        <div class="w-14 h-14 bg-white/[0.04] ring-1 ring-white/10 flex items-center justify-center text-[var(--color-faint)]">
          <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.25" aria-hidden="true">
            <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </div>
        <div class="space-y-1.5">
          <h2 class="text-base font-semibold tracking-tight">No profiles yet</h2>
          <p class="text-sm text-[var(--color-muted)] max-w-sm">Create an encode profile to define codec, CRF, preset, and x265 parameters.</p>
        </div>
        <Button onclick={openCreate}>New profile</Button>
      </div>
    {:else}
      <div class="overflow-hidden border border-[var(--color-border)] bg-[var(--color-surface)]">
        <ul class="divide-y divide-[var(--color-border)]/60">
          {#each profiles as p (p.id)}
            <li class="flex items-start gap-4 px-5 py-4 hover:bg-white/[0.02] transition-colors duration-200">
              <!-- Name + badges -->
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2 mb-1.5 flex-wrap">
                  <span class="text-sm font-medium text-[var(--color-text)]">{p.name}</span>
                  {#if p.is_builtin}
                    <span class="inline-flex items-center px-2.5 py-0.5 text-[11px] font-medium bg-[rgb(var(--accent-rgb)/0.12)] text-[var(--color-accent)] ring-1 ring-[var(--color-accent)]/25">Built-in</span>
                  {/if}
                  {#if p.parent_id != null}
                    <span class="inline-flex items-center bg-white/[0.06] px-2.5 py-0.5 text-[11px] font-medium text-[var(--color-text-dim)] ring-1 ring-white/10">
                      extends {parentName(p.parent_id)}
                    </span>
                  {/if}
                </div>

                <!-- Knobs (show only defined ones, null = inherited) -->
                <div class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-[var(--color-muted)]">
                  {#if p.codec}<span><span class="text-[var(--color-faint)]">codec</span> {p.codec}</span>{/if}
                  {#if p.crf != null}<span><span class="text-[var(--color-faint)]">crf</span> {p.crf}</span>{/if}
                  {#if p.preset}<span><span class="text-[var(--color-faint)]">preset</span> {p.preset}</span>{/if}
                  {#if p.audio}<span><span class="text-[var(--color-faint)]">audio</span> {p.audio}</span>{/if}
                  {#if p.scale != null}<span><span class="text-[var(--color-faint)]">scale</span> {p.scale}p</span>{/if}
                  {#if p.output_resolutions?.length}<span><span class="text-[var(--color-faint)]">res</span> {p.output_resolutions.join(', ')}p</span>{/if}
                  {#if p.psy_rd != null}<span><span class="text-[var(--color-faint)]">psy-rd</span> {p.psy_rd}</span>{/if}
                  {#if p.aq_mode != null}<span><span class="text-[var(--color-faint)]">aq-mode</span> {p.aq_mode}</span>{/if}
                  {#if p.bit_depth != null}<span><span class="text-[var(--color-faint)]">bit-depth</span> {p.bit_depth}</span>{/if}
                  {#if p.audio_languages?.length}<span><span class="text-[var(--color-faint)]">audio-lang</span> {p.audio_languages.map(languageName).join(', ')}</span>{/if}
                  {#if p.subtitle_languages?.length}<span><span class="text-[var(--color-faint)]">sub-lang</span> {p.subtitle_languages.map(languageName).join(', ')}</span>{/if}
                  {#if p.smartblur}<span class="text-[var(--color-info)]">smartblur</span>{/if}
                  {#if p.deinterlace}<span class="text-[var(--color-info)]">deinterlace</span>{/if}
                  {#if p.deband}<span class="text-[var(--color-info)]">deband</span>{/if}
                  {#if p.burn_subs}<span class="text-[var(--color-info)]">hardsub</span>{/if}
                </div>
              </div>

              <!-- Actions -->
              <div class="flex gap-1 shrink-0">
                <Button variant="ghost" size="icon" onclick={() => showResolved(p)} title="View resolved config">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
                    <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" stroke-linecap="round" stroke-linejoin="round"/>
                    <circle cx="12" cy="12" r="3"/>
                  </svg>
                </Button>
                {#if !p.is_builtin}
                  <Button variant="ghost" size="icon" onclick={() => openEdit(p)} title="Edit profile">
                    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
                      <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" stroke-linecap="round" stroke-linejoin="round"/>
                      <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" stroke-linecap="round" stroke-linejoin="round"/>
                    </svg>
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    onclick={() => deleteProfile(p.id)}
                    disabled={deleting === p.id}
                    title="Delete profile"
                    class="hover:text-[var(--color-error)]"
                  >
                    {#if deleting === p.id}
                      <Spinner size={12} />
                    {:else}
                      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
                        <polyline points="3 6 5 6 21 6" stroke-linecap="round" stroke-linejoin="round"/>
                        <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" stroke-linecap="round" stroke-linejoin="round"/>
                      </svg>
                    {/if}
                  </Button>
                {/if}
              </div>
            </li>
          {/each}
        </ul>
      </div>
    {/if}
  </div>
</div>

<!--
  Dual-mode language control. Mode options depend on container:
    MKV → "All languages" (wildcard) · "Specific" (multi-select chips)
    MP4 → "Default track"  (wildcard) · "Specific language" (single-select)
  Wildcard radio writes null; Specific writes the codes array (see langField).
-->
{#snippet langControl(kind: 'audio' | 'subtitle', mode: LangMode, field: 'audio_langs' | 'subtitle_langs')}
  <div class="space-y-2.5">
    <div class="flex flex-col gap-1.5">
      <label class="flex items-center gap-2 cursor-pointer text-sm text-[var(--color-text)]">
        {#if kind === 'audio'}
          <input type="radio" value="wildcard" bind:group={form.audio_lang_mode} class="accent-[var(--accent)]" />
        {:else}
          <input type="radio" value="wildcard" bind:group={form.subtitle_lang_mode} class="accent-[var(--accent)]" />
        {/if}
        <span>{isMp4 ? 'Default track' : 'All languages'}</span>
      </label>
      <label class="flex items-center gap-2 cursor-pointer text-sm text-[var(--color-text)]">
        {#if kind === 'audio'}
          <input type="radio" value="specific" bind:group={form.audio_lang_mode} class="accent-[var(--accent)]" />
        {:else}
          <input type="radio" value="specific" bind:group={form.subtitle_lang_mode} class="accent-[var(--accent)]" />
        {/if}
        <span>{isMp4 ? 'Specific language' : 'Specific'}</span>
      </label>
    </div>

    {#if mode === 'specific'}
      {#if isMp4}
        <!-- MP4: single-track pick → single-select dropdown -->
        <select
          value={form[field][0] ?? ''}
          onchange={(e) => { form[field] = e.currentTarget.value ? [e.currentTarget.value] : [] }}
          class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors cursor-pointer"
        >
          <option value="">— select a language —</option>
          {#each COMMON_LANGUAGES as l (l.code)}
            <option value={l.code}>{l.name}</option>
          {/each}
        </select>
      {:else}
        <!-- MKV: multi-select chips -->
        <div class="flex flex-wrap gap-2">
          {#each COMMON_LANGUAGES as l (l.code)}
            {@const selected = form[field].includes(l.code)}
            <button
              type="button"
              onclick={() => toggleLang(field, l.code)}
              aria-pressed={selected}
              class="px-2.5 py-1 text-xs font-medium ring-1 transition-colors duration-150 {selected
                ? 'bg-[rgb(var(--accent-rgb)/0.12)] text-[var(--color-accent)] ring-[var(--color-accent)]/40'
                : 'bg-white/[0.03] text-[var(--color-muted)] ring-white/10 hover:text-[var(--color-text)]'}"
            >
              {l.name}
            </button>
          {/each}
        </div>
      {/if}
    {/if}
  </div>
{/snippet}

<!-- Create / Edit Profile Modal -->
<Modal bind:open={editOpen} title={editMode === 'create' ? 'New profile' : 'Edit profile'}>
  {#snippet footer()}
    <Button variant="ghost" onclick={() => { editOpen = false }}>Cancel</Button>
    <Button onclick={save} disabled={saving || !form.name.trim()}>
      {#if saving}<Spinner size={14} />{/if}
      {editMode === 'create' ? 'Create' : 'Save'}
    </Button>
  {/snippet}

  <div class="space-y-5">
    <!-- Identity -->
    <fieldset class="space-y-4 border border-[var(--color-border)] bg-[var(--color-surface-2)] p-4">
      <legend class="px-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Identity</legend>
      <div>
        <label for="prof-name" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Name *</label>
        <input id="prof-name" type="text" bind:value={form.name} placeholder="My custom profile" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
      </div>
      <div>
        <label for="prof-parent" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Inherit from (parent)</label>
        <select
          id="prof-parent"
          bind:value={form.parent_id}
          class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors cursor-pointer"
        >
          <option value="">— none —</option>
          {#each parentOptions as p (p.id)}
            <option value={String(p.id)}>{p.name}{p.is_builtin ? ' (built-in)' : ''}</option>
          {/each}
        </select>
        {#if form.parent_id}
          <p class="text-xs text-[var(--color-muted)] mt-1">Unset fields below are inherited from the parent.</p>
        {/if}
      </div>
    </fieldset>

    <!-- Codec & Quality -->
    <fieldset class="space-y-4 border border-[var(--color-border)] bg-[var(--color-surface-2)] p-4">
      <legend class="px-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Codec &amp; Quality</legend>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="prof-crf" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">CRF</label>
          <input
            id="prof-crf"
            type="number"
            bind:value={form.crf}
            min="0"
            max="51"
            step="1"
            placeholder="inherit"
            class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)]/50 focus:outline-none focus:border-[var(--accent)] transition-colors"
          />
        </div>
        <div>
          <label for="prof-preset" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Preset</label>
          <select
            id="prof-preset"
            bind:value={form.preset}
            class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors cursor-pointer"
          >
            <option value="">inherit</option>
            {#each presets as p}
              <option value={p}>{p}</option>
            {/each}
          </select>
        </div>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="prof-bit-depth" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Bit depth</label>
          <select
            id="prof-bit-depth"
            bind:value={form.bit_depth}
            class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors cursor-pointer"
          >
            <option value="">inherit</option>
            <option value="8">8-bit</option>
            <option value="10">10-bit</option>
          </select>
        </div>
        <div>
          <label for="prof-resolutions" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Output resolutions (comma-separated)</label>
          <input id="prof-resolutions" type="text" bind:value={form.output_resolutions} placeholder="1080, 720, 480" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
        </div>
      </div>
    </fieldset>

    <!-- x265 Psychovisual -->
    <fieldset class="space-y-4 border border-[var(--color-border)] bg-[var(--color-surface-2)] p-4">
      <legend class="px-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">x265 Psychovisual</legend>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="prof-psy-rd" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">psy-rd</label>
          <input id="prof-psy-rd" type="text" bind:value={form.psy_rd} placeholder="inherit" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
        </div>
        <div>
          <label for="prof-psy-rdoq" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">psy-rdoq</label>
          <input id="prof-psy-rdoq" type="text" bind:value={form.psy_rdoq} placeholder="inherit" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
        </div>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="prof-aq-mode" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">aq-mode</label>
          <input
            id="prof-aq-mode"
            type="number"
            bind:value={form.aq_mode}
            min="0"
            max="4"
            placeholder="inherit"
            class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)]/50 focus:outline-none focus:border-[var(--accent)] transition-colors"
          />
        </div>
        <div>
          <label for="prof-aq-strength" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">aq-strength</label>
          <input id="prof-aq-strength" type="text" bind:value={form.aq_strength} placeholder="inherit" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
        </div>
      </div>
      <div>
        <label for="prof-deblock" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">deblock</label>
        <input id="prof-deblock" type="text" bind:value={form.deblock} placeholder="inherit, e.g. -1,-1" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] font-mono transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
      </div>
      <div>
        <label for="prof-x265-params" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">x265-params (raw)</label>
        <input id="prof-x265-params" type="text" bind:value={form.x265_params} placeholder="inherit" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] font-mono transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
      </div>
    </fieldset>

    <!-- Filters -->
    <fieldset class="space-y-3 border border-[var(--color-border)] bg-[var(--color-surface-2)] p-4">
      <legend class="px-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Filters</legend>
      <div class="flex gap-5 flex-wrap">
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" bind:checked={form.smartblur} class="border-[var(--color-border-strong)] accent-[var(--accent)]" />
          <span class="text-sm text-[var(--color-text)]">Smartblur</span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" bind:checked={form.deinterlace} class="border-[var(--color-border-strong)] accent-[var(--accent)]" />
          <span class="text-sm text-[var(--color-text)]">Deinterlace (yadif)</span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" bind:checked={form.deband} class="border-[var(--color-border-strong)] accent-[var(--accent)]" />
          <span class="text-sm text-[var(--color-text)]">Deband</span>
        </label>
      </div>
    </fieldset>

    <!-- Codec, Audio & Container -->
    <fieldset class="space-y-4 border border-[var(--color-border)] bg-[var(--color-surface-2)] p-4">
      <legend class="px-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Codec, Audio &amp; Container</legend>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="prof-codec" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Codec</label>
          <select
            id="prof-codec"
            bind:value={form.codec}
            class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors cursor-pointer"
          >
            <option value="">inherit</option>
            {#each codecs as c}
              <option value={c.value}>{c.label}</option>
            {/each}
          </select>
        </div>
        <div>
          <label for="prof-container" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Container</label>
          <select
            id="prof-container"
            bind:value={form.container}
            class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors cursor-pointer"
          >
            <option value="">inherit</option>
            <option value="mkv">MKV</option>
            <option value="mp4">MP4</option>
          </select>
        </div>
      </div>

      <div>
        <label for="prof-audio" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Audio codec</label>
        <input id="prof-audio" type="text" bind:value={form.audio} placeholder="copy / aac / flac" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
      </div>

      {#if isMp4}
        <p class="flex items-start gap-2 text-xs text-[var(--color-info)]">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true" class="mt-px shrink-0">
            <circle cx="12" cy="12" r="10"/>
            <path d="M12 16v-4M12 8h.01" stroke-linecap="round"/>
          </svg>
          <span>Subtitles are burned in (hardsub); audio is converted to AAC.</span>
        </p>
        <p class="text-[11px] text-[var(--color-faint)]">Burn subtitles <span class="text-[var(--color-text-dim)]">on</span> (derived from container)</p>
      {:else if form.container === 'mkv'}
        <p class="text-[11px] text-[var(--color-faint)]">Burn subtitles <span class="text-[var(--color-text-dim)]">off</span> — tracks are soft-copied (derived from container)</p>
      {/if}

      <!-- Audio languages -->
      <div class="space-y-2">
        <span class="block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Audio languages</span>
        {#if form.container === ''}
          <p class="text-xs text-[var(--color-muted)]">Select a container to configure language tracks.</p>
        {:else}
          {@render langControl('audio', form.audio_lang_mode, 'audio_langs')}
        {/if}
      </div>

      <!-- Subtitle languages -->
      <div class="space-y-2">
        <span class="block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Subtitle languages</span>
        {#if form.container === ''}
          <p class="text-xs text-[var(--color-muted)]">Select a container to configure language tracks.</p>
        {:else}
          {@render langControl('subtitle', form.subtitle_lang_mode, 'subtitle_langs')}
        {/if}
      </div>
    </fieldset>

    {#if formError}
      <p class="text-sm text-[var(--color-error)]">{formError}</p>
    {/if}
  </div>
</Modal>

<!-- Resolved profile modal -->
<Modal bind:open={resolvedOpen} title="Resolved config — {resolvedName}">
  {#snippet footer()}
    <Button variant="ghost" onclick={() => { resolvedOpen = false }}>Close</Button>
  {/snippet}

  {#if loadingResolved}
    <div class="flex items-center justify-center py-8">
      <Spinner size={24} />
    </div>
  {:else if resolved}
    <div class="overflow-hidden border border-[var(--color-border)] bg-[var(--color-surface-2)]">
      <dl class="divide-y divide-[var(--color-border)]/60 font-mono text-xs">
        {#each Object.entries(resolved) as [k, v]}
          <div class="flex justify-between gap-4 px-4 py-2">
            <dt class="text-[var(--color-muted)]">{k}</dt>
            <dd class="text-[var(--color-text)] text-right break-all">{Array.isArray(v) ? v.join(', ') : String(v)}</dd>
          </div>
        {/each}
      </dl>
    </div>
  {/if}
</Modal>
