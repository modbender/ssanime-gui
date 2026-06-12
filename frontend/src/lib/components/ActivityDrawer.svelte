<script lang="ts">
  // Right-anchored Activity drawer. Two modes in one panel:
  //   list  — currently-active episodes grouped by series (fill-sweep cards)
  //   detail — one episode's stage progress, source path + reveal, outputs + reveal
  // Mounted once globally in App.svelte (mirrors the welcome/source-gate modals).
  import { api, type EpisodeDetail, type OutputSummary } from '$lib/api'
  import { activityState, closeDrawer, backToList, selectEpisode } from '$lib/activity.svelte'
  import { activeBySeries, episodeStage, liveStatus } from '$lib/pipeline.svelte'
  import { sseState } from '$lib/sse.svelte'
  import { formatBytes, statusColor } from '$lib/utils'
  import EpisodeFill from '$lib/components/EpisodeFill.svelte'
  import Button from '$lib/components/Button.svelte'
  import Spinner from '$lib/components/Spinner.svelte'

  const inDetail = $derived(activityState.selectedEpisodeId != null)
  const groups = $derived(activeBySeries(activityState.activeEpisodes))

  // ---- Detail mode: fetch the single episode, keep live fields from the store ----
  let detail = $state<EpisodeDetail | null>(null)
  let detailLoading = $state(false)
  let detailError = $state('')

  // Reveal feedback (inline, lightweight — no toast framework).
  let revealMsg = $state<{ kind: 'ok' | 'err'; text: string } | null>(null)
  let revealBusy = $state<string | null>(null) // 'source' | `out:${id}`

  let loadedId = $state<number | null>(null)
  $effect(() => {
    const id = activityState.selectedEpisodeId
    if (id == null) {
      detail = null
      loadedId = null
      return
    }
    if (id === loadedId) return
    loadedId = id
    detail = null
    detailError = ''
    revealMsg = null
    detailLoading = true
    api
      .getEpisode(id)
      .then((d) => { detail = d })
      .catch((e: any) => { detailError = e?.message ?? 'Could not load episode.' })
      .finally(() => { detailLoading = false })
  })

  // Live download tick + per-output live encode percent overlay the fetched row.
  const dl = $derived(detail ? (sseState.downloadProgress[detail.id] ?? null) : null)
  function outPercent(o: OutputSummary): number | null {
    return sseState.encodeProgress[o.id]?.percent ?? null
  }
  function outStatus(o: OutputSummary): string {
    return sseState.outputStatus[o.id] ?? o.status
  }
  const detailStatus = $derived(detail ? liveStatus(detail) : '')

  async function revealSource() {
    if (!detail) return
    revealBusy = 'source'
    revealMsg = null
    try {
      await api.revealEpisodeSource(detail.id)
      revealMsg = { kind: 'ok', text: 'Opened in file explorer.' }
    } catch (e: any) {
      revealMsg = { kind: 'err', text: revealError(e) }
    } finally {
      revealBusy = null
    }
  }

  async function revealOut(o: OutputSummary) {
    revealBusy = `out:${o.id}`
    revealMsg = null
    try {
      await api.revealOutput(o.id)
      revealMsg = { kind: 'ok', text: 'Opened in file explorer.' }
    } catch (e: any) {
      revealMsg = { kind: 'err', text: revealError(e) }
    } finally {
      revealBusy = null
    }
  }

  function revealError(e: any): string {
    const msg = String(e?.message ?? '')
    if (msg.includes('409')) return 'File was cleaned up or moved.'
    if (msg.includes('404')) return 'File path is not set yet.'
    if (msg.includes('403')) return 'File is outside the managed folders.'
    return msg || 'Could not open the file.'
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (inDetail) backToList()
      else closeDrawer()
    }
  }
</script>

<svelte:window onkeydown={activityState.open ? onKeydown : undefined} />

{#if activityState.open}
  <!-- Backdrop (click to dismiss) -->
  <button
    type="button"
    aria-label="Close activity"
    tabindex="-1"
    class="fixed inset-0 z-40 cursor-default bg-black/55 backdrop-blur-[2px] animate-fade"
    onclick={closeDrawer}
  ></button>

  <!-- Panel -->
  <aside
    class="fixed inset-y-0 right-0 z-50 flex w-full max-w-[400px] flex-col border-l border-[var(--color-border)] bg-[var(--color-surface)] shadow-[-30px_0_80px_-20px_rgba(0,0,0,0.7)] ss-slide-in"
    aria-label="Activity"
  >
    <!-- Header -->
    <div class="flex shrink-0 items-center gap-2 border-b border-[var(--color-border)] px-4 py-3.5">
      {#if inDetail}
        <button
          type="button"
          onclick={backToList}
          class="-ml-1 flex items-center justify-center p-1.5 text-[var(--color-muted)] transition-colors hover:bg-white/5 hover:text-[var(--color-text)]"
          aria-label="Back to activity"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25"><path d="M19 12H5M12 5l-7 7 7 7" stroke-linecap="round" stroke-linejoin="round"/></svg>
        </button>
      {:else}
        <span class="flex h-2 w-2 shrink-0 items-center justify-center">
          <span class="h-2 w-2 rounded-full {groups.length ? 'bg-[var(--color-success)] animate-pulse' : 'bg-[var(--color-faint)]'}"></span>
        </span>
      {/if}
      <h2 class="text-sm font-semibold tracking-tight text-[var(--color-text)]">
        {inDetail ? (detail?.series_title || 'Episode') : 'Activity'}
      </h2>
      {#if !inDetail && groups.length}
        <span class="text-xs font-medium tabular-nums text-[var(--color-muted)]">
          {activityState.activeEpisodes.length} active
        </span>
      {/if}
      <button
        type="button"
        onclick={closeDrawer}
        class="ml-auto flex items-center justify-center p-1.5 text-[var(--color-muted)] transition-colors hover:bg-white/5 hover:text-[var(--color-text)]"
        aria-label="Close activity"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6 6 18M6 6l12 12" stroke-linecap="round"/></svg>
      </button>
    </div>

    <!-- Body -->
    <div class="min-h-0 flex-1 overflow-y-auto">
      {#if inDetail}
        <!-- ─── Detail mode ───────────────────────────────────────── -->
        {#if detailLoading}
          <div class="flex h-40 items-center justify-center text-[var(--color-muted)]"><Spinner size={24} /></div>
        {:else if detailError}
          <div class="px-4 py-10 text-center text-sm text-[var(--color-error)]">{detailError}</div>
        {:else if detail}
          {@const dStage = episodeStage(detail)}
          <div class="space-y-5 p-4">
            <!-- Episode header -->
            <div>
              <div class="flex items-center gap-2">
                <span class="text-[11px] font-bold tabular-nums text-[var(--color-text-dim)]">
                  {detail.episode_no != null ? `E${String(detail.episode_no).padStart(2, '0')}` : 'SP'}
                </span>
                <span class="text-xs font-medium {statusColor(detailStatus)}">{detailStatus}</span>
              </div>
              <p class="mt-1 text-[15px] font-semibold leading-snug text-[var(--color-text)]">
                {detail.title ?? (detail.episode_no != null ? `Episode ${detail.episode_no}` : 'Special')}
              </p>
            </div>

            <!-- Download progress -->
            {#if dl || dStage === 'downloading' || detail.source_size != null}
              <section class="space-y-2">
                <h3 class="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Download</h3>
                <div class="h-1.5 w-full overflow-hidden bg-white/[0.06]">
                  <div
                    class="h-full bg-[var(--accent-text)] transition-[width] duration-500"
                    style="width: {dl?.percent ?? (dStage === 'downloading' ? 0 : 100)}%"
                  ></div>
                </div>
                <div class="flex justify-between text-[11px] tabular-nums text-[var(--color-muted)]">
                  {#if dl}
                    <span>{formatBytes(dl.bytes_done)} / {formatBytes(dl.bytes_total)}</span>
                    <span>{formatBytes(dl.speed_bps)}/s · {dl.peers} peers</span>
                  {:else}
                    <span>{detail.source_size != null ? formatBytes(detail.source_size) : '—'}</span>
                    <span>{dStage === 'downloading' ? 'starting…' : 'complete'}</span>
                  {/if}
                </div>
              </section>
            {/if}

            <!-- Outputs / encode rows -->
            {#if detail.outputs.length}
              <section class="space-y-2">
                <h3 class="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Outputs</h3>
                <ul class="space-y-2">
                  {#each detail.outputs as o (o.id)}
                    {@const oStatus = outStatus(o)}
                    {@const oPct = outPercent(o)}
                    <li class="border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2.5">
                      <div class="flex items-center justify-between gap-2">
                        <div class="flex items-center gap-2">
                          <span class="text-xs font-semibold tabular-nums text-[var(--color-text)]">{o.resolution}p</span>
                          <span class="text-[11px] font-medium {statusColor(oStatus)}">{oStatus}</span>
                        </div>
                        <span class="text-[11px] tabular-nums text-[var(--color-muted)]">
                          {#if oStatus === 'encoding' && oPct != null}{Math.round(oPct)}%{:else if o.encoded_size != null}{formatBytes(o.encoded_size)}{/if}
                        </span>
                      </div>
                      {#if oStatus === 'encoding' && oPct != null}
                        <div class="mt-1.5 h-1 w-full overflow-hidden bg-white/[0.06]">
                          <div class="h-full bg-[var(--accent-text)] transition-[width] duration-500" style="width: {oPct}%"></div>
                        </div>
                      {/if}
                      {#if o.error_message}
                        <p class="mt-1 truncate text-[11px] text-[var(--color-error)]" title={o.error_message}>{o.error_message}</p>
                      {/if}
                      {#if o.encoded_path}
                        <div class="mt-2 flex items-center gap-2">
                          <code class="min-w-0 flex-1 truncate text-[10px] text-[var(--color-faint)]" title={o.encoded_path}>{o.encoded_path}</code>
                          <Button size="sm" variant="secondary" onclick={() => revealOut(o)} disabled={revealBusy === `out:${o.id}`}>
                            {#if revealBusy === `out:${o.id}`}<Spinner size={11} />{:else}{@render RevealIcon()}{/if}
                            Open
                          </Button>
                        </div>
                      {/if}
                    </li>
                  {/each}
                </ul>
              </section>
            {/if}

            <!-- Source path + reveal -->
            <section class="space-y-2">
              <h3 class="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--color-muted)]">Source file</h3>
              {#if detail.source_cleaned_at != null}
                <p class="flex items-center gap-1.5 text-xs text-[var(--color-success)]">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M20 6 9 17l-5-5" stroke-linecap="round" stroke-linejoin="round"/></svg>
                  Source cleaned up after archive.
                </p>
              {:else if detail.source_path}
                <div class="flex items-center gap-2">
                  <code class="min-w-0 flex-1 truncate text-[10px] text-[var(--color-faint)]" title={detail.source_path}>{detail.source_path}</code>
                  <Button size="sm" variant="secondary" onclick={revealSource} disabled={revealBusy === 'source'}>
                    {#if revealBusy === 'source'}<Spinner size={11} />{:else}{@render RevealIcon()}{/if}
                    Open
                  </Button>
                </div>
              {:else}
                <p class="text-xs text-[var(--color-muted)]">No source file yet.</p>
              {/if}
            </section>

            {#if revealMsg}
              <p class="text-xs {revealMsg.kind === 'err' ? 'text-[var(--color-error)]' : 'text-[var(--color-success)]'}">{revealMsg.text}</p>
            {/if}
          </div>
        {/if}
      {:else}
        <!-- ─── List mode ─────────────────────────────────────────── -->
        {#if groups.length === 0}
          <div class="flex h-full flex-col items-center justify-center gap-3 px-6 py-16 text-center">
            <div class="flex h-12 w-12 items-center justify-center bg-white/[0.04] text-[var(--color-faint)] ring-1 ring-white/10">
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M13 2 3 14h7l-1 8 10-12h-7l1-8z" stroke-linecap="round" stroke-linejoin="round"/></svg>
            </div>
            <div class="space-y-1">
              <h3 class="text-sm font-semibold text-[var(--color-text)]">Nothing active</h3>
              <p class="text-xs text-[var(--color-muted)]">Downloads and encodes in progress show up here live.</p>
            </div>
          </div>
        {:else}
          <div class="space-y-5 p-4">
            {#each groups as g (g.seriesId)}
              <section class="space-y-2">
                <h3 class="truncate px-0.5 text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--color-muted)]" title={g.seriesTitle}>
                  {g.seriesTitle}
                </h3>
                <div class="space-y-2">
                  {#each g.episodes as ep (ep.id)}
                    <EpisodeFill {ep} compact onclick={() => selectEpisode(ep.id)} />
                  {/each}
                </div>
              </section>
            {/each}
          </div>
        {/if}
      {/if}
    </div>
  </aside>
{/if}

{#snippet RevealIcon()}
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z" stroke-linecap="round" stroke-linejoin="round"/></svg>
{/snippet}

<style>
  @keyframes ss-slide-in {
    from { transform: translateX(100%); }
    to { transform: translateX(0); }
  }
  .ss-slide-in {
    animation: ss-slide-in 0.32s cubic-bezier(0.32, 0.72, 0, 1) both;
  }
  @media (prefers-reduced-motion: reduce) {
    .ss-slide-in { animation-duration: 0.01ms; }
  }
</style>
