<script lang="ts">
  import { api, type Feed } from '$lib/api'
  import Badge from '$lib/components/Badge.svelte'
  import Button from '$lib/components/Button.svelte'
  import Input from '$lib/components/Input.svelte'
  import Modal from '$lib/components/Modal.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import { formatDate } from '$lib/utils'

  let feeds = $state<Feed[]>([])
  let loading = $state(true)
  let error = $state('')

  // Modal state
  let editOpen = $state(false)
  let editMode = $state<'create' | 'edit'>('create')
  let saving = $state(false)
  let deleting = $state<number | null>(null)

  // Form state — all text fields are plain strings to satisfy Input bind:value
  type FeedForm = {
    url: string
    type: string
    site: string
    enabled: boolean
    interval_seconds: number
    offset_seconds: number
    title_regex: string
    extra_tags: string
    quality: number | null
    subtype: string | null
    deinterlace: boolean
    uncensored: boolean
    bluray: boolean
    series_id_str: string
  }

  let form = $state<FeedForm>({
    url: '',
    type: 'rss',
    site: '',
    enabled: true,
    interval_seconds: 3600,
    offset_seconds: 0,
    title_regex: '',
    extra_tags: '',
    quality: null,
    subtype: null,
    deinterlace: false,
    uncensored: false,
    bluray: false,
    series_id_str: '',
  })
  let editingId = $state<number | null>(null)

  async function load() {
    loading = true
    error = ''
    try {
      feeds = await api.listFeeds()
    } catch (e: any) {
      error = e.message
    } finally {
      loading = false
    }
  }

  $effect(() => { load() })

  function emptyForm(): FeedForm {
    return {
      url: '',
      type: 'rss',
      site: '',
      enabled: true,
      interval_seconds: 3600,
      offset_seconds: 0,
      title_regex: '',
      extra_tags: '',
      quality: null,
      subtype: null,
      deinterlace: false,
      uncensored: false,
      bluray: false,
      series_id_str: '',
    }
  }

  function openCreate() {
    editMode = 'create'
    editingId = null
    form = emptyForm()
    editOpen = true
  }

  function openEdit(feed: Feed) {
    editMode = 'edit'
    editingId = feed.id
    form = {
      url: feed.url,
      type: feed.type,
      site: feed.site ?? '',
      enabled: feed.enabled,
      interval_seconds: feed.interval_seconds,
      offset_seconds: feed.offset_seconds,
      title_regex: feed.title_regex ?? '',
      extra_tags: feed.extra_tags ?? '',
      quality: feed.quality,
      subtype: feed.subtype ?? null,
      deinterlace: feed.deinterlace,
      uncensored: feed.uncensored,
      bluray: feed.bluray,
      series_id_str: String(feed.series_id),
    } satisfies FeedForm
    editOpen = true
  }

  async function save() {
    if (!form.url.trim()) return
    saving = true
    try {
      const body: Partial<Feed> = {
        url: form.url,
        type: form.type,
        site: form.site || undefined,
        enabled: form.enabled,
        interval_seconds: form.interval_seconds,
        offset_seconds: form.offset_seconds,
        title_regex: form.title_regex || null,
        extra_tags: form.extra_tags || null,
        quality: form.quality,
        subtype: form.subtype,
        deinterlace: form.deinterlace,
        uncensored: form.uncensored,
        bluray: form.bluray,
        series_id: form.series_id_str ? Number(form.series_id_str) : undefined,
      }
      if (editMode === 'create') {
        await api.createFeed(body)
      } else if (editingId != null) {
        await api.patchFeed(editingId, body)
      }
      editOpen = false
      await load()
    } catch (e: any) {
      alert(e.message)
    } finally {
      saving = false
    }
  }

  async function toggleEnabled(feed: Feed) {
    try {
      const updated = await api.patchFeed(feed.id, { enabled: !feed.enabled })
      feeds = feeds.map(f => f.id === feed.id ? updated : f)
    } catch (e: any) {
      alert(e.message)
    }
  }

  async function deleteFeed(id: number) {
    if (!confirm('Delete this feed?')) return
    deleting = id
    try {
      await api.deleteFeed(id)
      feeds = feeds.filter(f => f.id !== id)
    } catch (e: any) {
      alert(e.message)
    } finally {
      deleting = null
    }
  }

  function intervalLabel(secs: number) {
    if (secs < 60) return `${secs}s`
    if (secs < 3600) return `${secs / 60}m`
    if (secs < 86400) return `${secs / 3600}h`
    return `${secs / 86400}d`
  }
</script>

<div class="flex flex-col h-full">
  <!-- Header -->
  <div class="flex items-center justify-between px-6 py-4 border-b border-[#2a2a35]">
    <h1 class="text-lg font-semibold text-[#e8e8f0]">Auto-downloader</h1>
    <Button onclick={openCreate}>
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
        <path d="M12 5v14M5 12h14" stroke-linecap="round"/>
      </svg>
      Add feed
    </Button>
  </div>

  <div class="flex-1 overflow-y-auto px-6 py-5">
    {#if loading}
      <div class="flex items-center justify-center h-64">
        <Spinner size={28} />
      </div>
    {:else if error}
      <div class="flex items-center justify-center h-64 text-red-400 text-sm">{error}</div>
    {:else if feeds.length === 0}
      <div class="flex flex-col items-center justify-center h-64 gap-3">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="#2a2a35" stroke-width="1.5">
          <path d="M15 17h5l-1.405-1.405A2.032 2.032 0 0 1 18 14.158V11a6.002 6.002 0 0 0-4-5.659V5a2 2 0 1 0-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 1 1-6 0v-1m6 0H9" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        <p class="text-[#6b6b80] text-sm">No feeds configured. Add one to start auto-downloading.</p>
        <Button onclick={openCreate}>Add feed</Button>
      </div>
    {:else}
      <div class="space-y-2">
        {#each feeds as feed (feed.id)}
          <div class="rounded-xl border border-[#2a2a35] bg-[#111118] p-4 flex items-start gap-4">
            <!-- Enable toggle -->
            <button
              class="mt-0.5 shrink-0 w-10 h-5 rounded-full transition-colors {feed.enabled ? 'bg-[#7c6af0]' : 'bg-[#2a2a35]'} relative focus-visible:outline-2 focus-visible:outline-[#7c6af0] cursor-pointer"
              onclick={() => toggleEnabled(feed)}
              role="switch"
              aria-checked={feed.enabled}
              title={feed.enabled ? 'Disable' : 'Enable'}
            >
              <span class="absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white transition-transform {feed.enabled ? 'translate-x-5' : 'translate-x-0'}"></span>
            </button>

            <!-- Info -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 mb-1 flex-wrap">
                <span class="text-[#e8e8f0] text-sm font-medium truncate">{feed.url}</span>
                <Badge class="bg-[#18181f] text-[#6b6b80] border border-[#2a2a35]">{feed.type.toUpperCase()}</Badge>
                {#if feed.site}
                  <Badge class="bg-[#18181f] text-[#6b6b80] border border-[#2a2a35]">{feed.site}</Badge>
                {/if}
                {#if !feed.enabled}
                  <Badge class="bg-[#18181f] text-[#6b6b80] border border-[#2a2a35]">Disabled</Badge>
                {/if}
              </div>
              <div class="flex items-center gap-3 text-xs text-[#6b6b80] flex-wrap">
                <span>Series #{feed.series_id}</span>
                <span>Every {intervalLabel(feed.interval_seconds)}</span>
                {#if feed.quality}
                  <span>{feed.quality}p</span>
                {/if}
                {#if feed.title_regex}
                  <span class="font-mono truncate max-w-xs" title={feed.title_regex}>/{feed.title_regex}/</span>
                {/if}
                {#if feed.uncensored}
                  <span class="text-yellow-400">Uncensored</span>
                {/if}
                {#if feed.bluray}
                  <span class="text-blue-400">Blu-ray</span>
                {/if}
                {#if feed.last_checked}
                  <span>Last checked {formatDate(feed.last_checked)}</span>
                {/if}
              </div>
            </div>

            <!-- Actions -->
            <div class="flex gap-1 shrink-0">
              <Button variant="ghost" size="icon" onclick={() => openEdit(feed)} title="Edit">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                  <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" stroke-linecap="round" stroke-linejoin="round"/>
                  <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
              </Button>
              <Button
                variant="ghost"
                size="icon"
                onclick={() => deleteFeed(feed.id)}
                disabled={deleting === feed.id}
                title="Delete"
                class="hover:text-red-400"
              >
                {#if deleting === feed.id}
                  <Spinner size={12} />
                {:else}
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                    <polyline points="3 6 5 6 21 6" stroke-linecap="round" stroke-linejoin="round"/>
                    <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" stroke-linecap="round" stroke-linejoin="round"/>
                    <path d="M10 11v6M14 11v6M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2" stroke-linecap="round" stroke-linejoin="round"/>
                  </svg>
                {/if}
              </Button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>

<!-- Create/Edit Feed Modal -->
<Modal bind:open={editOpen} title={editMode === 'create' ? 'Add feed' : 'Edit feed'}>
  {#snippet footer()}
    <Button variant="ghost" onclick={() => { editOpen = false }}>Cancel</Button>
    <Button onclick={save} disabled={saving || !form.url?.trim()}>
      {#if saving}<Spinner size={14} />{/if}
      {editMode === 'create' ? 'Add' : 'Save'}
    </Button>
  {/snippet}

  <div class="space-y-4">
    <div>
      <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">Feed URL *</label>
      <Input bind:value={form.url} placeholder="https://nyaa.si/?page=rss&q=..." />
    </div>

    <div class="grid grid-cols-2 gap-3">
      <div>
        <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">Type</label>
        <select
          bind:value={form.type}
          class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] focus:outline-none focus:border-[#7c6af0] cursor-pointer"
        >
          <option value="rss">RSS</option>
          <option value="scrape">Scrape</option>
        </select>
      </div>
      <div>
        <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">Site</label>
        <Input bind:value={form.site} placeholder="nyaa, anidex, …" />
      </div>
    </div>

    <div>
      <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">Series ID</label>
      <Input bind:value={form.series_id_str} type="text" placeholder="Series ID number" />
    </div>

    <div class="grid grid-cols-2 gap-3">
      <div>
        <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">Quality (p)</label>
        <select
          bind:value={form.quality}
          class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] focus:outline-none focus:border-[#7c6af0] cursor-pointer"
        >
          <option value={null}>Any</option>
          <option value={2160}>2160p</option>
          <option value={1080}>1080p</option>
          <option value={720}>720p</option>
          <option value={480}>480p</option>
        </select>
      </div>
      <div>
        <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">Interval (s)</label>
        <input
          type="number"
          bind:value={form.interval_seconds}
          min="60"
          step="60"
          class="w-full h-9 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-sm text-[#e8e8f0] focus:outline-none focus:border-[#7c6af0]"
        />
      </div>
    </div>

    <div>
      <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">Title regex filter</label>
      <Input bind:value={form.title_regex} placeholder="e.g. \[SubGroup\]" class="font-mono" />
    </div>

    <div>
      <label class="block text-xs text-[#6b6b80] mb-1.5 uppercase tracking-wide">Extra tags</label>
      <Input bind:value={form.extra_tags} placeholder="Extra torrent tags to match" />
    </div>

    <div class="flex gap-4 flex-wrap">
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="checkbox" bind:checked={form.uncensored} class="rounded border-[#2a2a35] accent-[#7c6af0]" />
        <span class="text-sm text-[#e8e8f0]">Uncensored</span>
      </label>
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="checkbox" bind:checked={form.bluray} class="rounded border-[#2a2a35] accent-[#7c6af0]" />
        <span class="text-sm text-[#e8e8f0]">Blu-ray</span>
      </label>
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="checkbox" bind:checked={form.deinterlace} class="rounded border-[#2a2a35] accent-[#7c6af0]" />
        <span class="text-sm text-[#e8e8f0]">Deinterlace</span>
      </label>
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="checkbox" bind:checked={form.enabled} class="rounded border-[#2a2a35] accent-[#7c6af0]" />
        <span class="text-sm text-[#e8e8f0]">Enabled</span>
      </label>
    </div>
  </div>
</Modal>
