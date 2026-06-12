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
      case 'error': return 'text-[var(--color-error)]'
      case 'warn': return 'text-[var(--color-warning)]'
      case 'debug': return 'text-[var(--color-muted)]'
      default: return 'text-[var(--color-text)]'
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
  <!-- Page header -->
  <div class="sticky top-0 z-10 flex items-center justify-between px-6 sm:px-10 py-4 border-b border-[var(--color-border)] bg-[var(--color-bg)]/95 backdrop-blur-md">
    <div class="flex items-center gap-2.5">
      <h1 class="text-[15px] font-semibold tracking-tight">Logs</h1>
      <div
        class="w-2 h-2 rounded-full shrink-0 {sseState.connected ? 'bg-[var(--color-success)] animate-pulse' : 'bg-[var(--color-border-strong)]'}"
        title={sseState.connected ? 'Live' : 'Disconnected'}
        aria-label={sseState.connected ? 'Connected — live updates active' : 'Disconnected'}
        role="status"
      ></div>
    </div>
    <div class="flex items-center gap-2">
      <label for="log-level-filter" class="sr-only">Filter by log level</label>
      <select
        id="log-level-filter"
        bind:value={filterLevel}
        class="h-8 border border-[var(--color-border)] bg-[var(--color-surface)] px-3 text-xs text-[var(--color-text)] focus:outline-none focus:border-[var(--accent)] transition-colors cursor-pointer"
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
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
          <path d="M12 19V5M5 12l7 7 7-7" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        Auto-scroll
      </Button>
      <Button variant="outline" size="sm" onclick={loadHistory} disabled={loading} title="Reload log history">
        {#if loading}
          <Spinner size={12} />
        {:else}
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
            <path d="M1 4v6h6M23 20v-6h-6" stroke-linecap="round" stroke-linejoin="round"/>
            <path d="M20.49 9A9 9 0 0 0 5.64 5.64L1 10m22 4l-4.64 4.36A9 9 0 0 1 3.51 15" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        {/if}
        Reload
      </Button>
    </div>
  </div>

  <!-- Log stream -->
  <div
    bind:this={logContainer}
    onscroll={onScroll}
    class="flex-1 overflow-y-auto no-scrollbar bg-[var(--color-surface)] font-mono text-xs leading-5"
    role="log"
    aria-live="polite"
    aria-label="Application log stream"
  >
    {#if loading && historicLines.length === 0}
      <div class="flex items-center justify-center h-32 text-[var(--color-muted)]">
        <Spinner size={20} />
      </div>
    {:else if error}
      <div class="text-[var(--color-error)] px-6 sm:px-10 py-3">{error}</div>
    {:else if filtered.length === 0}
      <div class="flex flex-col items-center justify-center gap-3 py-20 text-center px-6">
        <div class="w-12 h-12 bg-white/[0.04] ring-1 ring-white/10 flex items-center justify-center text-[var(--color-faint)]">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">
            <path d="M9 12h6m-6 4h6m2 5H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5.586a1 1 0 0 1 .707.293l5.414 5.414a1 1 0 0 1 .293.707V19a2 2 0 0 1-2 2z" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </div>
        <p class="text-sm text-[var(--color-muted)]">
          {filterLevel ? `No ${filterLevel} entries.` : 'No log entries yet. Waiting for activity…'}
        </p>
      </div>
    {:else}
      <div class="px-4 sm:px-6 py-2">
        {#each filtered as line (line.key)}
          <div class="flex gap-3 py-px hover:bg-white/[0.025] px-2 group">
            {#if line.ts}
              <span class="text-[var(--color-faint)] shrink-0 select-none w-20 tabular-nums">{formatTs(line.ts)}</span>
            {/if}
            <span class={levelColor(line.level)}>{line.text}</span>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <!-- Footer status bar -->
  <div class="px-6 sm:px-10 py-2.5 border-t border-[var(--color-border)] bg-[var(--color-surface-2)] text-xs text-[var(--color-muted)] flex items-center gap-3">
    <span class="tabular-nums">{filtered.length} line{filtered.length !== 1 ? 's' : ''}</span>
    {#if filterLevel}
      <span class="text-[var(--color-faint)]">·</span>
      <span>filtered: {filterLevel}</span>
    {/if}
    <span class="ml-auto tabular-nums">{sseState.logs.length} live events buffered</span>
  </div>
</div>
