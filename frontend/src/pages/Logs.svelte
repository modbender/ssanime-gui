<script lang="ts">
  import { api } from '$lib/api'
  import { sseState, type LogEvent } from '$lib/sse.svelte'
  import Button from '$lib/components/Button.svelte'
  import Spinner from '$lib/components/Spinner.svelte'

  let historicLines = $state<string[]>([])
  let loading = $state(true)
  let error = $state('')
  let filterLevel = $state('')
  let autoScroll = $state(true)
  let logContainer = $state<HTMLElement | null>(null)

  async function loadHistory() {
    loading = true
    error = ''
    try {
      const res = await api.getLogs()
      historicLines = res.lines
    } catch (e: any) {
      error = e.message
    } finally {
      loading = false
    }
  }

  $effect(() => { loadHistory() })

  // Merge historic lines + live SSE logs into a unified display list
  // SSE logs are newest-first (prepended), so reverse them for display
  const liveEvents = $derived([...sseState.logs].reverse())

  // Combined: historic first, then live appended
  const allLines = $derived<Array<{ key: string; text: string; level: string; ts: number | null }>>([
    ...historicLines.map((line, i) => ({
      key: `h-${i}`,
      text: line,
      level: guessLevel(line),
      ts: null,
    })),
    ...liveEvents.map((e, i) => ({
      key: `l-${i}-${e.ts}`,
      text: `${levelTag(e.level)} ${e.message}`,
      level: e.level,
      ts: e.ts,
    })),
  ])

  const filtered = $derived(
    filterLevel
      ? allLines.filter(l => l.level === filterLevel)
      : allLines
  )

  function guessLevel(line: string): string {
    const l = line.toLowerCase()
    if (l.includes('[error]') || l.includes('error') || l.includes('err')) return 'error'
    if (l.includes('[warn]') || l.includes('warning') || l.includes('warn')) return 'warn'
    if (l.includes('[debug]') || l.includes('debug')) return 'debug'
    return 'info'
  }

  function levelTag(level: string): string {
    switch (level) {
      case 'error': return '[ERR]'
      case 'warn': return '[WRN]'
      case 'debug': return '[DBG]'
      default: return '[INF]'
    }
  }

  function levelColor(level: string): string {
    switch (level) {
      case 'error': return 'text-red-400'
      case 'warn': return 'text-yellow-400'
      case 'debug': return 'text-[#6b6b80]'
      default: return 'text-[#e8e8f0]'
    }
  }

  function formatTs(ts: number | null): string {
    if (!ts) return ''
    const d = new Date(ts * 1000)
    return d.toLocaleTimeString()
  }

  // Auto-scroll to bottom when new lines arrive
  $effect(() => {
    const _ = filtered.length
    if (autoScroll && logContainer) {
      logContainer.scrollTop = logContainer.scrollHeight
    }
  })

  function onScroll() {
    if (!logContainer) return
    const atBottom = logContainer.scrollHeight - logContainer.scrollTop - logContainer.clientHeight < 40
    autoScroll = atBottom
  }
</script>

<div class="flex flex-col h-full">
  <!-- Header -->
  <div class="flex items-center justify-between px-6 py-4 border-b border-[#2a2a35]">
    <div class="flex items-center gap-3">
      <h1 class="text-lg font-semibold text-[#e8e8f0]">Logs</h1>
      <div class="w-2 h-2 rounded-full {sseState.connected ? 'bg-green-400 animate-pulse' : 'bg-[#2a2a35]'}" title={sseState.connected ? 'Live' : 'Disconnected'}></div>
    </div>
    <div class="flex items-center gap-2">
      <select
        bind:value={filterLevel}
        class="h-8 rounded-lg border border-[#2a2a35] bg-[#111118] px-3 text-xs text-[#e8e8f0] focus:outline-none focus:border-[#7c6af0] cursor-pointer"
      >
        <option value="">All levels</option>
        <option value="debug">Debug</option>
        <option value="info">Info</option>
        <option value="warn">Warn</option>
        <option value="error">Error</option>
      </select>
      <Button
        variant={autoScroll ? 'default' : 'outline'}
        size="sm"
        onclick={() => { autoScroll = !autoScroll }}
        title="Toggle auto-scroll"
      >
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
          <path d="M12 19V5M5 12l7 7 7-7" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        Auto-scroll
      </Button>
      <Button variant="outline" size="sm" onclick={loadHistory} disabled={loading}>
        {#if loading}<Spinner size={12} />{:else}
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
            <path d="M1 4v6h6M23 20v-6h-6" stroke-linecap="round" stroke-linejoin="round"/>
            <path d="M20.49 9A9 9 0 0 0 5.64 5.64L1 10m22 4l-4.64 4.36A9 9 0 0 1 3.51 15" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        {/if}
        Reload
      </Button>
    </div>
  </div>

  <!-- Log output -->
  <div
    bind:this={logContainer}
    onscroll={onScroll}
    class="flex-1 overflow-y-auto px-4 py-3 font-mono text-xs leading-5 bg-[#0a0a0f]"
  >
    {#if loading && historicLines.length === 0}
      <div class="flex items-center justify-center h-32">
        <Spinner size={20} />
      </div>
    {:else if error}
      <div class="text-red-400 px-2 py-1">{error}</div>
    {:else if filtered.length === 0}
      <div class="text-[#6b6b80] px-2 py-4 text-center">
        {filterLevel ? `No ${filterLevel} entries.` : 'No log entries yet. Waiting for activity…'}
      </div>
    {:else}
      {#each filtered as line (line.key)}
        <div class="flex gap-2 py-px hover:bg-[#111118]/60 px-2 rounded group">
          {#if line.ts}
            <span class="text-[#6b6b80]/60 shrink-0 select-none w-20">{formatTs(line.ts)}</span>
          {/if}
          <span class={levelColor(line.level)}>{line.text}</span>
        </div>
      {/each}
    {/if}
  </div>

  <!-- Footer: line count -->
  <div class="px-6 py-2 border-t border-[#2a2a35] text-xs text-[#6b6b80] flex items-center gap-3">
    <span>{filtered.length} line{filtered.length !== 1 ? 's' : ''}</span>
    {#if filterLevel}
      <span>· filtered: {filterLevel}</span>
    {/if}
    <span class="ml-auto">{sseState.logs.length} live events buffered</span>
  </div>
</div>
