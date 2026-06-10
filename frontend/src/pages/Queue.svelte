<script lang="ts">
  import { api, type EpisodeDetail } from '$lib/api'
  import Badge from '$lib/components/Badge.svelte'
  import Button from '$lib/components/Button.svelte'
  import ProgressBar from '$lib/components/ProgressBar.svelte'
  import Spinner from '$lib/components/Spinner.svelte'
  import { sseState } from '$lib/sse.svelte'
  import { statusColor, formatBytes } from '$lib/utils'

  let downloading = $state<EpisodeDetail[]>([])
  let encoding = $state<EpisodeDetail[]>([])
  let loading = $state(true)
  let error = $state('')

  async function load() {
    loading = true
    error = ''
    try {
      const q = await api.getQueue()
      downloading = q.downloading
      encoding = q.encoding
    } catch (e: any) {
      error = e.message
    } finally {
      loading = false
    }
  }

  $effect(() => { load() })

  // Refresh queue when SSE status events arrive
  $effect(() => {
    const _ = Object.keys(sseState.episodeStatus).length
    if (!loading) load()
  })

  function downloadProgress(ep: EpisodeDetail) {
    return sseState.downloadProgress[ep.id] ?? null
  }

  function encodeProgress(ep: EpisodeDetail) {
    return sseState.encodeProgress[ep.id] ?? null
  }

  function liveStatus(ep: EpisodeDetail) {
    return sseState.episodeStatus[ep.id] ?? ep.status
  }

  const totalItems = $derived(downloading.length + encoding.length)
</script>

<div class="flex flex-col h-full overflow-y-auto">
  <!-- Page header -->
  <div class="sticky top-0 z-10 flex items-center justify-between px-6 sm:px-10 py-4 border-b border-[var(--color-border)] bg-[var(--color-bg)]/95 backdrop-blur-md">
    <div class="flex items-baseline gap-2.5">
      <h1 class="text-[15px] font-semibold tracking-tight">Queue</h1>
      {#if !loading && totalItems > 0}
        <span class="text-xs font-medium tabular-nums px-2 py-0.5 rounded-full bg-[rgb(var(--accent-rgb)/0.14)] text-[var(--color-accent)]">{totalItems} active</span>
      {/if}
    </div>
    <Button variant="outline" size="sm" onclick={load} disabled={loading} title="Refresh queue">
      {#if loading}
        <Spinner size={12} />
      {:else}
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
          <path d="M1 4v6h6M23 20v-6h-6" stroke-linecap="round" stroke-linejoin="round"/>
          <path d="M20.49 9A9 9 0 0 0 5.64 5.64L1 10m22 4l-4.64 4.36A9 9 0 0 1 3.51 15" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      {/if}
      Refresh
    </Button>
  </div>

  <div class="flex-1 px-6 sm:px-10 py-8 space-y-8 animate-fade-up">
    {#if loading}
      <div class="flex items-center justify-center h-64 text-[var(--color-muted)]">
        <Spinner size={28} />
      </div>
    {:else if error}
      <div class="flex items-center justify-center h-64 text-[var(--color-error)] text-sm">{error}</div>
    {:else if totalItems === 0}
      <!-- Empty state -->
      <div class="flex flex-col items-center justify-center gap-4 py-24 text-center">
        <div class="w-14 h-14 rounded-2xl bg-white/[0.04] ring-1 ring-white/10 flex items-center justify-center text-[var(--color-faint)]">
          <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.25" aria-hidden="true">
            <path d="M4 6h16M4 10h16M4 14h16M4 18h16" stroke-linecap="round"/>
          </svg>
        </div>
        <div class="space-y-1.5">
          <h2 class="text-base font-semibold tracking-tight">Queue is empty</h2>
          <p class="text-sm text-[var(--color-muted)] max-w-sm">Nothing is downloading or encoding right now. Select episodes from a series to begin.</p>
        </div>
      </div>
    {:else}
      <!-- Downloading section -->
      {#if downloading.length > 0}
        <section class="animate-fade-up">
          <div class="flex items-baseline gap-2.5 mb-4">
            <div class="w-2 h-2 rounded-full bg-[var(--color-info)] animate-pulse shrink-0 self-center" aria-hidden="true"></div>
            <h2 class="text-[13px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Downloading</h2>
            <span class="text-xs font-medium tabular-nums text-[var(--color-muted)]">{downloading.length}</span>
          </div>
          <div class="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--color-border)] bg-[var(--color-surface)]">
            <ul class="divide-y divide-[var(--color-border)]/60">
              {#each downloading as ep (ep.id)}
                {@const prog = downloadProgress(ep)}
                {@const status = liveStatus(ep)}
                <li class="px-5 py-4">
                  <div class="flex items-start justify-between gap-3 mb-3">
                    <div class="min-w-0 flex-1">
                      <p class="text-sm font-medium truncate text-[var(--color-text)]">
                        {ep.title ?? `Episode ${ep.episode_no}`}
                      </p>
                      <div class="flex items-center gap-2 mt-1 flex-wrap">
                        {#if ep.episode_no != null}
                          <span class="rounded-md bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[11px] font-semibold tabular-nums ring-1 ring-[var(--color-border)] text-[var(--color-text-dim)]">E{String(ep.episode_no).padStart(2, '0')}</span>
                        {/if}
                        {#if ep.release_group}
                          <span class="text-[var(--color-muted)] text-xs">[{ep.release_group}]</span>
                        {/if}
                        {#if ep.source_size}
                          <span class="text-[var(--color-faint)] text-xs tabular-nums">{formatBytes(ep.source_size)}</span>
                        {/if}
                      </div>
                    </div>
                    <span class="text-xs font-medium shrink-0 {statusColor(status)}">{status}</span>
                  </div>

                  {#if prog}
                    <div class="space-y-1.5">
                      <ProgressBar value={prog.percent} max={100} />
                      <div class="flex justify-between text-xs text-[var(--color-muted)]">
                        <span class="tabular-nums">{formatBytes(prog.bytes_done)} / {formatBytes(prog.bytes_total)}</span>
                        <span class="tabular-nums">{formatBytes(prog.speed_bps)}/s · {prog.percent}%</span>
                      </div>
                    </div>
                  {:else}
                    <ProgressBar value={0} max={100} />
                  {/if}
                </li>
              {/each}
            </ul>
          </div>
        </section>
      {/if}

      <!-- Encoding section -->
      {#if encoding.length > 0}
        <section class="animate-fade-up">
          <div class="flex items-baseline gap-2.5 mb-4">
            <div class="w-2 h-2 rounded-full bg-[var(--color-accent)] animate-pulse shrink-0 self-center" aria-hidden="true"></div>
            <h2 class="text-[13px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Encoding</h2>
            <span class="text-xs font-medium tabular-nums text-[var(--color-muted)]">{encoding.length}</span>
          </div>
          <div class="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--color-border)] bg-[var(--color-surface)]">
            <ul class="divide-y divide-[var(--color-border)]/60">
              {#each encoding as ep (ep.id)}
                {@const prog = encodeProgress(ep)}
                {@const status = liveStatus(ep)}
                <li class="px-5 py-4">
                  <div class="flex items-start justify-between gap-3 mb-3">
                    <div class="min-w-0 flex-1">
                      <p class="text-sm font-medium truncate text-[var(--color-text)]">
                        {ep.title ?? `Episode ${ep.episode_no}`}
                      </p>
                      <div class="flex items-center gap-2 mt-1 flex-wrap">
                        {#if ep.episode_no != null}
                          <span class="rounded-md bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[11px] font-semibold tabular-nums ring-1 ring-[var(--color-border)] text-[var(--color-text-dim)]">E{String(ep.episode_no).padStart(2, '0')}</span>
                        {/if}
                        {#if prog}
                          <span class="text-[var(--color-muted)] text-xs tabular-nums">{prog.resolution}p</span>
                          {#if prog.speed}
                            <span class="text-[var(--color-muted)] text-xs tabular-nums">{prog.speed}</span>
                          {/if}
                        {/if}
                      </div>
                    </div>
                    <span class="text-xs font-medium shrink-0 {statusColor(status)}">{status}</span>
                  </div>

                  {#if prog}
                    <div class="space-y-1.5">
                      <ProgressBar value={prog.percent} max={100} />
                      <div class="flex justify-between text-xs text-[var(--color-muted)]">
                        <span class="tabular-nums">{prog.percent}% complete</span>
                        {#if prog.speed}
                          <span class="tabular-nums">{prog.speed}</span>
                        {/if}
                      </div>
                    </div>
                  {:else}
                    <ProgressBar value={0} max={100} />
                  {/if}

                  <!-- Outputs -->
                  {#if ep.outputs.length > 0}
                    <div class="flex gap-1 mt-3 flex-wrap">
                      {#each ep.outputs as out (out.id)}
                        <span
                          class="rounded-md bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[11px] font-medium tabular-nums ring-1 ring-[var(--color-border)] {statusColor(out.status)}"
                          title={out.error_message ?? out.status}
                        >
                          {out.resolution}p
                        </span>
                      {/each}
                    </div>
                  {/if}
                </li>
              {/each}
            </ul>
          </div>
        </section>
      {/if}
    {/if}
  </div>
</div>
