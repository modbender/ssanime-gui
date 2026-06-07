<script lang="ts">
  import { api, type Profile, type ResolvedProfile } from '$lib/api'
  import Badge from '$lib/components/Badge.svelte'
  import Button from '$lib/components/Button.svelte'
  import Input from '$lib/components/Input.svelte'
  import Modal from '$lib/components/Modal.svelte'
  import Spinner from '$lib/components/Spinner.svelte'

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
      alert(e.message)
    } finally {
      saving = false
    }
  }

  async function deleteProfile(id: number) {
    if (!confirm('Delete this profile?')) return
    deleting = id
    try {
      await api.deleteProfile(id)
      profiles = profiles.filter(p => p.id !== id)
    } catch (e: any) {
      alert(e.message)
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
      alert(e.message)
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

<div class="flex flex-col h-full">
  <!-- Header -->
  <div class="flex items-center justify-between px-6 py-4 border-b border-[#2a2a35]">
    <h1 class="text-lg font-semibold text-[#e8e8f0]">Encode profiles</h1>
    <Button onclick={openCreate}>
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
        <path d="M12 5v14M5 12h14" stroke-linecap="round"/>
      </svg>
      New profile
    </Button>
  </div>

  <div class="flex-1 overflow-y-auto px-6 py-5">
    {#if loading}
      <div class="flex items-center justify-center h-64">
        <Spinner size={28} />
      </div>
    {:else if error}
      <div class="flex items-center justify-center h-64 text-red-400 text-sm">{error}</div>
    {:else if profiles.length === 0}
      <div class="flex flex-col items-center justify-center h-64 gap-3">
        <p class="text-[#6b6b80] text-sm">No profiles yet.</p>
        <Button onclick={openCreate}>New profile</Button>
      </div>
    {:else}
      <div class="space-y-2">
        {#each profiles as p (p.id)}
          <div class="rounded-xl border border-[#2a2a35] bg-[#111118] p-4 flex items-start gap-4">
            <!-- Name + badges -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 mb-1.5 flex-wrap">
                <span class="text-[#e8e8f0] text-sm font-medium">{p.name}</span>
                {#if p.is_builtin}
                  <Badge class="bg-[#7c6af0]/15 text-[#7c6af0] border border-[#7c6af0]/30">Built-in</Badge>
                {/if}
                {#if p.parent_id != null}
                  <Badge class="bg-[#18181f] text-[#6b6b80] border border-[#2a2a35]">
                    extends {parentName(p.parent_id)}
                  </Badge>
                {/if}
              </div>

              <!-- Knobs (show only defined ones, null = inherited) -->
              <div class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-[#6b6b80]">
                {#if p.codec}<span><span class="text-[#6b6b80]/60">codec</span> {p.codec}</span>{/if}
                {#if p.crf != null}<span><span class="text-[#6b6b80]/60">crf</span> {p.crf}</span>{/if}
                {#if p.preset}<span><span class="text-[#6b6b80]/60">preset</span> {p.preset}</span>{/if}
                {#if p.audio}<span><span class="text-[#6b6b80]/60">audio</span> {p.audio}</span>{/if}
                {#if p.scale != null}<span><span class="text-[#6b6b80]/60">scale</span> {p.scale}p</span>{/if}
                {#if p.output_resolutions?.length}<span><span class="text-[#6b6b80]/60">res</span> {p.output_resolutions.join(', ')}p</span>{/if}
                {#if p.psy_rd != null}<span><span class="text-[#6b6b80]/60">psy-rd</span> {p.psy_rd}</span>{/if}
                {#if p.aq_mode != null}<span><span class="text-[#6b6b80]/60">aq-mode</span> {p.aq_mode}</span>{/if}
                {#if p.smartblur}<span class="text-cyan-400">smartblur</span>{/if}
                {#if p.deinterlace}<span class="text-cyan-400">deinterlace</span>{/if}
              </div>
            </div>

            <!-- Actions -->
            <div class="flex gap-1 shrink-0">
              <Button variant="ghost" size="icon" onclick={() => showResolved(p)} title="View resolved config">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                  <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" stroke-linecap="round" stroke-linejoin="round"/>
                  <circle cx="12" cy="12" r="3"/>
                </svg>
              </Button>
              {#if !p.is_builtin}
                <Button variant="ghost" size="icon" onclick={() => openEdit(p)} title="Edit">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                    <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" stroke-linecap="round" stroke-linejoin="round"/>
                    <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" stroke-linecap="round" stroke-linejoin="round"/>
                  </svg>
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  onclick={() => deleteProfile(p.id)}
                  disabled={deleting === p.id}
                  title="Delete"
                  class="hover:text-red-400"
                >
                  {#if deleting === p.id}
                    <Spinner size={12} />
                  {:else}
                    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                      <polyline points="3 6 5 6 21 6" stroke-linecap="round" stroke-linejoin="round"/>
                      <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" stroke-linecap="round" stroke-linejoin="round"/>
                    </svg>
                  {/if}
                </Button>
              {/if}
            </div>
          </div>
        {/each}
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

  <div class="space-y-4">
    <div>
      <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">Name *</label>
      <Input bind:value={form.name} placeholder="My custom profile" />
    </div>

    <div>
      <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">Inherit from (parent)</label>
      <select
        bind:value={form.parent_id}
        class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] focus:outline-none focus:border-[#7c6af0] cursor-pointer"
      >
        <option value="">— none —</option>
        {#each parentOptions as p (p.id)}
          <option value={String(p.id)}>{p.name}{p.is_builtin ? ' (built-in)' : ''}</option>
        {/each}
      </select>
      {#if form.parent_id}
        <p class="text-xs text-[#6b6b80] mt-1">Unset fields below are inherited from the parent.</p>
      {/if}
    </div>

    <div class="grid grid-cols-2 gap-3">
      <div>
        <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">CRF</label>
        <input
          type="number"
          bind:value={form.crf}
          min="0"
          max="51"
          step="1"
          placeholder="inherit"
          class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] placeholder:text-[#6b6b80]/50 focus:outline-none focus:border-[#7c6af0]"
        />
      </div>
      <div>
        <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">Preset</label>
        <select
          bind:value={form.preset}
          class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] focus:outline-none focus:border-[#7c6af0] cursor-pointer"
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
        <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">Audio</label>
        <Input bind:value={form.audio} placeholder="copy / aac / flac" />
      </div>
      <div>
        <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">Container</label>
        <Input bind:value={form.container} placeholder="mkv / mp4" />
      </div>
    </div>

    <div>
      <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">Output resolutions (comma-separated)</label>
      <Input bind:value={form.output_resolutions} placeholder="1080, 720, 480" />
    </div>

    <div class="grid grid-cols-2 gap-3">
      <div>
        <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">psy-rd</label>
        <Input bind:value={form.psy_rd} placeholder="inherit" type="text" />
      </div>
      <div>
        <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">psy-rdoq</label>
        <Input bind:value={form.psy_rdoq} placeholder="inherit" type="text" />
      </div>
    </div>

    <div class="grid grid-cols-2 gap-3">
      <div>
        <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">aq-mode</label>
        <input
          type="number"
          bind:value={form.aq_mode}
          min="0"
          max="4"
          placeholder="inherit"
          class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] placeholder:text-[#6b6b80]/50 focus:outline-none focus:border-[#7c6af0]"
        />
      </div>
      <div>
        <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">aq-strength</label>
        <Input bind:value={form.aq_strength} placeholder="inherit" type="text" />
      </div>
    </div>

    <div>
      <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">deblock</label>
      <Input bind:value={form.deblock} placeholder="inherit, e.g. -1,-1" class="font-mono" />
    </div>

    <div>
      <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">x265-params (raw)</label>
      <Input bind:value={form.x265_params} placeholder="inherit" class="font-mono" />
    </div>

    <div class="flex gap-5 flex-wrap">
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="checkbox" bind:checked={form.smartblur} class="rounded border-[#2a2a35] accent-[#7c6af0]" />
        <span class="text-sm text-[#e8e8f0]">Smartblur</span>
      </label>
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="checkbox" bind:checked={form.deinterlace} class="rounded border-[#2a2a35] accent-[#7c6af0]" />
        <span class="text-sm text-[#e8e8f0]">Deinterlace (yadif)</span>
      </label>
    </div>
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
    <div class="space-y-1 font-mono text-xs">
      {#each Object.entries(resolved) as [k, v]}
        <div class="flex justify-between gap-4 py-1 border-b border-[#2a2a35]/50">
          <span class="text-[#6b6b80]">{k}</span>
          <span class="text-[#e8e8f0] text-right break-all">{Array.isArray(v) ? v.join(', ') : String(v)}</span>
        </div>
      {/each}
    </div>
  {/if}
</Modal>
