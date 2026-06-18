<script lang="ts">
  import { api, type Profile, type ResolvedProfile } from '$lib/api'
  import Badge from '$lib/components/Badge.svelte'
  import Button from '$lib/components/Button.svelte'
  import Input from '$lib/components/Input.svelte'
  import Modal from '$lib/components/Modal.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import { toast } from '$lib/toast.svelte'
  import { confirm } from '$lib/confirm.svelte'
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
    smartblur: boolean | null
    deinterlace: boolean | null
    output_resolutions: string // comma-separated
  }

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
    smartblur: null,
    deinterlace: null,
    output_resolutions: '',
  })

  async function load() {
    loading = true
    error = ''
    try {
      profiles = await api.listProfiles()
    } catch (e: any) {
      error = e.message
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
      smartblur: null,
      deinterlace: null,
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
      smartblur: p.smartblur ?? null,
      deinterlace: p.deinterlace ?? null,
      output_resolutions: p.output_resolutions?.join(', ') ?? '',
    }
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
      smartblur: f.smartblur,
      deinterlace: f.deinterlace,
      output_resolutions: f.output_resolutions
        ? f.output_resolutions.split(',').map(s => Number(s.trim())).filter(Boolean)
        : null,
    }
  }

  function openCreate() {
    editMode = 'create'
    editingId = null
    form = emptyForm()
    editOpen = true
  }

  function openEdit(p: Profile) {
    editMode = 'edit'
    editingId = p.id
    form = profileToForm(p)
    editOpen = true
  }

  async function save() {
    if (!form.name.trim()) return
    saving = true
    try {
      const body = formToBody(form)
      if (editMode === 'create') {
        await api.createProfile(body)
      } else if (editingId != null) {
        await api.patchProfile(editingId, body)
      }
      editOpen = false
      await load()
    } catch (e: any) {
      toast.error(e.message)
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
    } catch (e: any) {
      toast.error(e.message)
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
    } catch (e: any) {
      toast.error(e.message)
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
                  {#if p.smartblur}<span class="text-[var(--color-info)]">smartblur</span>{/if}
                  {#if p.deinterlace}<span class="text-[var(--color-info)]">deinterlace</span>{/if}
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
      <div>
        <label for="prof-resolutions" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Output resolutions (comma-separated)</label>
        <input id="prof-resolutions" type="text" bind:value={form.output_resolutions} placeholder="1080, 720, 480" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
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
      </div>
    </fieldset>

    <!-- Audio & Container -->
    <fieldset class="space-y-4 border border-[var(--color-border)] bg-[var(--color-surface-2)] p-4">
      <legend class="px-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Audio &amp; Container</legend>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="prof-audio" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Audio</label>
          <input id="prof-audio" type="text" bind:value={form.audio} placeholder="copy / aac / flac" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
        </div>
        <div>
          <label for="prof-container" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Container</label>
          <input id="prof-container" type="text" bind:value={form.container} placeholder="mkv / mp4" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
        </div>
      </div>
    </fieldset>
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
