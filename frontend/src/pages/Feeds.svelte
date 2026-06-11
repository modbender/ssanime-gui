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

<div class="flex flex-col h-full overflow-y-auto">
  <!-- Page header -->
  <div class="sticky top-0 z-10 flex items-center justify-between px-6 sm:px-10 py-4 border-b border-[var(--color-border)] bg-[var(--color-bg)]/95 backdrop-blur-md">
    <div class="flex items-baseline gap-2.5">
      <h1 class="text-[15px] font-semibold tracking-tight">Auto-downloader</h1>
      {#if !loading && feeds.length > 0}
        <span class="text-xs font-medium tabular-nums text-[var(--color-muted)]">{feeds.length}</span>
      {/if}
    </div>
    <Button onclick={openCreate}>
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
        <path d="M12 5v14M5 12h14" stroke-linecap="round"/>
      </svg>
      Add feed
    </Button>
  </div>

  <div class="flex-1 px-6 sm:px-10 py-8 animate-fade-up">
    {#if loading}
      <div class="flex items-center justify-center h-64 text-[var(--color-muted)]">
        <Spinner size={28} />
      </div>
    {:else if error}
      <div class="flex items-center justify-center h-64 text-[var(--color-error)] text-sm">{error}</div>
    {:else if feeds.length === 0}
      <!-- Empty state -->
      <div class="flex flex-col items-center justify-center gap-4 py-24 text-center">
        <div class="w-14 h-14 bg-white/[0.04] ring-1 ring-white/10 flex items-center justify-center text-[var(--color-faint)]">
          <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.25" aria-hidden="true">
            <path d="M15 17h5l-1.405-1.405A2.032 2.032 0 0 1 18 14.158V11a6.002 6.002 0 0 0-4-5.659V5a2 2 0 1 0-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 1 1-6 0v-1m6 0H9" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </div>
        <div class="space-y-1.5">
          <h2 class="text-base font-semibold tracking-tight">No feeds configured</h2>
          <p class="text-sm text-[var(--color-muted)] max-w-sm">Add an RSS or scrape feed to start auto-downloading new episodes as they air.</p>
        </div>
        <Button onclick={openCreate}>Add feed</Button>
      </div>
    {:else}
      <div class="overflow-hidden border border-[var(--color-border)] bg-[var(--color-surface)]">
        <ul class="divide-y divide-[var(--color-border)]/60">
          {#each feeds as feed (feed.id)}
            <li class="flex items-start gap-4 px-5 py-4 hover:bg-white/[0.02] transition-colors duration-200">
              <!-- Enable toggle -->
              <button
                class="mt-0.5 shrink-0 w-10 h-5 transition-colors duration-200 relative focus-visible:outline-2 focus-visible:outline-[var(--accent)] cursor-pointer {feed.enabled ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-border-strong)]'}"
                onclick={() => toggleEnabled(feed)}
                role="switch"
                aria-checked={feed.enabled}
                title={feed.enabled ? 'Disable feed' : 'Enable feed'}
                aria-label={feed.enabled ? 'Disable feed' : 'Enable feed'}
              >
                <span class="absolute top-0.5 left-0.5 w-4 h-4 bg-white transition-transform duration-200 {feed.enabled ? 'translate-x-5' : 'translate-x-0'}"></span>
              </button>

              <!-- Info -->
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2 mb-1 flex-wrap">
                  <span class="text-sm font-medium truncate text-[var(--color-text)]">{feed.url}</span>
                  <span class="inline-flex items-center bg-white/[0.06] px-2.5 py-0.5 text-[11px] font-medium text-[var(--color-text-dim)] ring-1 ring-white/10">{feed.type.toUpperCase()}</span>
                  {#if feed.site}
                    <span class="inline-flex items-center bg-white/[0.06] px-2.5 py-0.5 text-[11px] font-medium text-[var(--color-text-dim)] ring-1 ring-white/10">{feed.site}</span>
                  {/if}
                  {#if !feed.enabled}
                    <span class="inline-flex items-center bg-[var(--color-surface-2)] px-2.5 py-0.5 text-[11px] font-medium text-[var(--color-muted)] ring-1 ring-[var(--color-border)]">Disabled</span>
                  {/if}
                </div>
                <div class="flex items-center gap-3 text-xs text-[var(--color-muted)] flex-wrap">
                  <span>Series #{feed.series_id}</span>
                  <span>Every {intervalLabel(feed.interval_seconds)}</span>
                  {#if feed.quality}
                    <span class="tabular-nums">{feed.quality}p</span>
                  {/if}
                  {#if feed.title_regex}
                    <span class="font-mono truncate max-w-xs text-[var(--color-text-dim)]" title={feed.title_regex}>/{feed.title_regex}/</span>
                  {/if}
                  {#if feed.uncensored}
                    <span class="text-[var(--color-warning)]">Uncensored</span>
                  {/if}
                  {#if feed.bluray}
                    <span class="text-[var(--color-info)]">Blu-ray</span>
                  {/if}
                  {#if feed.last_checked}
                    <span>Last checked {formatDate(feed.last_checked)}</span>
                  {/if}
                </div>
              </div>

              <!-- Actions -->
              <div class="flex gap-1 shrink-0">
                <Button variant="ghost" size="icon" onclick={() => openEdit(feed)} title="Edit feed">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
                    <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" stroke-linecap="round" stroke-linejoin="round"/>
                    <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" stroke-linecap="round" stroke-linejoin="round"/>
                  </svg>
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  onclick={() => deleteFeed(feed.id)}
                  disabled={deleting === feed.id}
                  title="Delete feed"
                  class="hover:text-[var(--color-error)]"
                >
                  {#if deleting === feed.id}
                    <Spinner size={12} />
                  {:else}
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
      <label for="feed-url" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Feed URL *</label>
      <input id="feed-url" type="text" bind:value={form.url} placeholder="https://nyaa.si/?page=rss&q=..." class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
    </div>

    <div class="grid grid-cols-2 gap-3">
      <div>
        <label for="feed-type" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Type</label>
        <select
          id="feed-type"
          bind:value={form.type}
          class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors cursor-pointer"
        >
          <option value="rss">RSS</option>
          <option value="scrape">Scrape</option>
        </select>
      </div>
      <div>
        <label for="feed-site" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Site</label>
        <input id="feed-site" type="text" bind:value={form.site} placeholder="nyaa, anidex, …" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
      </div>
    </div>

    <div>
      <label for="feed-series-id" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Series ID</label>
      <input id="feed-series-id" type="text" bind:value={form.series_id_str} placeholder="Series ID number" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
    </div>

    <div class="grid grid-cols-2 gap-3">
      <div>
        <label for="feed-quality" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Quality (p)</label>
        <select
          id="feed-quality"
          bind:value={form.quality}
          class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors cursor-pointer"
        >
          <option value={null}>Any</option>
          <option value={2160}>2160p</option>
          <option value={1080}>1080p</option>
          <option value={720}>720p</option>
          <option value={480}>480p</option>
        </select>
      </div>
      <div>
        <label for="feed-interval" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Interval (s)</label>
        <input
          id="feed-interval"
          type="number"
          bind:value={form.interval_seconds}
          min="60"
          step="60"
          class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]"
        />
      </div>
    </div>

    <div>
      <label for="feed-regex" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Title regex filter</label>
      <input id="feed-regex" type="text" bind:value={form.title_regex} placeholder="e.g. \[SubGroup\]" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] font-mono transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
    </div>

    <div>
      <label for="feed-tags" class="mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]">Extra tags</label>
      <input id="feed-tags" type="text" bind:value={form.extra_tags} placeholder="Extra torrent tags to match" class="w-full h-9 border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] transition-colors duration-200 focus:outline-none focus:border-[var(--accent)] focus:bg-[var(--color-surface-2)]" />
    </div>

    <div class="flex gap-4 flex-wrap">
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="checkbox" bind:checked={form.uncensored} class="border-[var(--color-border-strong)] accent-[var(--accent)]" />
        <span class="text-sm text-[var(--color-text)]">Uncensored</span>
      </label>
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="checkbox" bind:checked={form.bluray} class="border-[var(--color-border-strong)] accent-[var(--accent)]" />
        <span class="text-sm text-[var(--color-text)]">Blu-ray</span>
      </label>
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="checkbox" bind:checked={form.deinterlace} class="border-[var(--color-border-strong)] accent-[var(--accent)]" />
        <span class="text-sm text-[var(--color-text)]">Deinterlace</span>
      </label>
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="checkbox" bind:checked={form.enabled} class="border-[var(--color-border-strong)] accent-[var(--accent)]" />
        <span class="text-sm text-[var(--color-text)]">Enabled</span>
      </label>
    </div>
  </div>
</Modal>
