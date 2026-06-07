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

<div class="flex flex-col h-full">
  <!-- Header -->
  <div class="flex items-center justify-between px-6 py-4 border-b border-[#2a2a35]">
    <div class="flex items-center gap-3">
      <h1 class="text-lg font-semibold text-[#e8e8f0]">Queue</h1>
      {#if !loading && totalItems > 0}
        <span class="text-xs px-2 py-0.5 rounded-full bg-[#7c6af0]/20 text-[#7c6af0] font-medium">{totalItems} active</span>
      {/if}
    </div>
    <Button variant="outline" size="sm" onclick={load} disabled={loading}>
      {#if loading}
        <Spinner size={12} />
      {:else}
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
          <path d="M1 4v6h6M23 20v-6h-6" stroke-linecap="round" stroke-linejoin="round"/>
          <path d="M20.49 9A9 9 0 0 0 5.64 5.64L1 10m22 4l-4.64 4.36A9 9 0 0 1 3.51 15" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      {/if}
      Refresh
    </Button>
  </div>

  <div class="flex-1 overflow-y-auto px-6 py-5 space-y-6">
    {#if loading}
      <div class="flex items-center justify-center h-64">
        <Spinner size={28} />
      </div>
    {:else if error}
      <div class="flex items-center justify-center h-64 text-red-400 text-sm">{error}</div>
    {:else if totalItems === 0}
      <div class="flex flex-col items-center justify-center h-64 gap-3">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="#2a2a35" stroke-width="1.5">
          <path d="M4 6h16M4 10h16M4 14h16M4 18h16" stroke-linecap="round"/>
        </svg>
        <p class="text-[#6b6b80] text-sm">Queue is empty. Nothing is downloading or encoding.</p>
      </div>
    {:else}
      <!-- Downloading section -->
      {#if downloading.length > 0}
        <section>
          <div class="flex items-center gap-2 mb-3">
            <div class="w-2 h-2 rounded-full bg-cyan-400 animate-pulse"></div>
            <h2 class="text-sm font-semibold text-[#e8e8f0] uppercase tracking-wide">Downloading</h2>
            <span class="text-xs text-[#6b6b80]">{downloading.length}</span>
          </div>
          <div class="space-y-2">
            {#each downloading as ep (ep.id)}
              {@const prog = downloadProgress(ep)}
              {@const status = liveStatus(ep)}
              <div class="rounded-xl border border-[#2a2a35] bg-[#111118] p-4">
                <div class="flex items-start justify-between gap-3 mb-3">
                  <div class="min-w-0">
                    <p class="text-[#e8e8f0] text-sm font-medium truncate">
                      {ep.title ?? `Episode ${ep.episode_no}`}
                    </p>
                    <div class="flex items-center gap-2 mt-0.5 flex-wrap">
                      {#if ep.episode_no != null}
                        <span class="text-[#6b6b80] text-xs">E{String(ep.episode_no).padStart(2, '0')}</span>
                      {/if}
                      {#if ep.release_group}
                        <span class="text-[#6b6b80] text-xs">[{ep.release_group}]</span>
                      {/if}
                      {#if ep.source_size}
                        <span class="text-[#6b6b80] text-xs">{formatBytes(ep.source_size)}</span>
                      {/if}
                    </div>
                  </div>
                  <span class="text-xs font-medium shrink-0 {statusColor(status)}">{status}</span>
                </div>

                {#if prog}
                  <div class="space-y-1.5">
                    <ProgressBar value={prog.percent} max={100} color="bg-cyan-400" />
                    <div class="flex justify-between text-xs text-[#6b6b80]">
                      <span>{formatBytes(prog.bytes_downloaded)} / {formatBytes(prog.bytes_total)}</span>
                      <span>{formatBytes(prog.speed_bps)}/s · {prog.percent}%</span>
                    </div>
                  </div>
                {:else}
                  <ProgressBar value={0} max={100} color="bg-cyan-400" />
                {/if}
              </div>
            {/each}
          </div>
        </section>
      {/if}

      <!-- Encoding section -->
      {#if encoding.length > 0}
        <section>
          <div class="flex items-center gap-2 mb-3">
            <div class="w-2 h-2 rounded-full bg-blue-400 animate-pulse"></div>
            <h2 class="text-sm font-semibold text-[#e8e8f0] uppercase tracking-wide">Encoding</h2>
            <span class="text-xs text-[#6b6b80]">{encoding.length}</span>
          </div>
          <div class="space-y-2">
            {#each encoding as ep (ep.id)}
              {@const prog = encodeProgress(ep)}
              {@const status = liveStatus(ep)}
              <div class="rounded-xl border border-[#2a2a35] bg-[#111118] p-4">
                <div class="flex items-start justify-between gap-3 mb-3">
                  <div class="min-w-0">
                    <p class="text-[#e8e8f0] text-sm font-medium truncate">
                      {ep.title ?? `Episode ${ep.episode_no}`}
                    </p>
                    <div class="flex items-center gap-2 mt-0.5 flex-wrap">
                      {#if ep.episode_no != null}
                        <span class="text-[#6b6b80] text-xs">E{String(ep.episode_no).padStart(2, '0')}</span>
                      {/if}
                      {#if prog}
                        <span class="text-[#6b6b80] text-xs">{prog.resolution}p</span>
                        <span class="text-[#6b6b80] text-xs">{prog.fps.toFixed(1)} fps</span>
                        <span class="text-[#6b6b80] text-xs">{prog.speed.toFixed(2)}x</span>
                      {/if}
                    </div>
                  </div>
                  <span class="text-xs font-medium shrink-0 {statusColor(status)}">{status}</span>
                </div>

                {#if prog}
                  <div class="space-y-1.5">
                    <ProgressBar value={prog.percent} max={100} color="bg-blue-400" />
                    <div class="flex justify-between text-xs text-[#6b6b80]">
                      <span>{prog.percent}% complete</span>
                      <span>{prog.speed.toFixed(2)}x speed</span>
                    </div>
                  </div>
                {:else}
                  <ProgressBar value={0} max={100} color="bg-blue-400" />
                {/if}

                <!-- Outputs -->
                {#if ep.outputs.length > 0}
                  <div class="flex gap-1 mt-3 flex-wrap">
                    {#each ep.outputs as out (out.id)}
                      <span class="text-xs px-1.5 py-0.5 rounded bg-[#18181f] border border-[#2a2a35] {statusColor(out.status)}">
                        {out.resolution}p
                      </span>
                    {/each}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        </section>
      {/if}
    {/if}
  </div>
</div>
