<script lang="ts">
  import { Link } from 'svelte-routing'
  import { sseState } from '$lib/sse.svelte'
  import { APP_NAME } from '$lib/app'
  import logoMark from '$lib/assets/logo-mark.svg?raw'

  const navItems = [
    { href: '/', label: 'Home', icon: 'M3 10.5 12 3l9 7.5M5 9v10a1 1 0 0 0 1 1h4v-6h4v6h4a1 1 0 0 0 1-1V9' },
    { href: '/library', label: 'Library', icon: 'M12 3v12m0 0 4-4m-4 4-4-4M5 21h14' },
    { href: '/activity', label: 'Activity', icon: 'M13 2 3 14h7l-1 8 10-12h-7l1-8z' },
    { href: '/profiles', label: 'Encode profiles', icon: 'M12 3 4 7v6c0 5 3.5 7.5 8 8.5 4.5-1 8-3.5 8-8.5V7l-8-4Z' },
    { href: '/extensions', label: 'Extensions', icon: 'M14 7h2.5A2.5 2.5 0 0 1 19 9.5V12h1a2 2 0 1 1 0 4h-1v2.5a2.5 2.5 0 0 1-2.5 2.5H14v-1a2 2 0 1 0-4 0v1H7.5A2.5 2.5 0 0 1 5 18.5V16H4a2 2 0 1 1 0-4h1V9.5A2.5 2.5 0 0 1 7.5 7H10V6a2 2 0 1 1 4 0v1Z' },
    { href: '/settings', label: 'Settings', icon: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z' },
    { href: '/logs', label: 'Logs', icon: 'M8 6h11M8 12h11M8 18h11M3.5 6h.01M3.5 12h.01M3.5 18h.01' },
  ]

  let currentPath = $state(window.location.pathname)

  function isActive(href: string) {
    if (href === '/') return currentPath === '/'
    return currentPath.startsWith(href)
  }

  $effect(() => {
    const handler = () => { currentPath = window.location.pathname }
    window.addEventListener('popstate', handler)
    return () => window.removeEventListener('popstate', handler)
  })
</script>

<aside class="relative flex flex-col items-center h-full w-[68px] shrink-0 bg-[var(--color-surface)] border-r border-[var(--color-border)] py-4 z-20">
  <!-- Logo mark -->
  <a
    href="/"
    onclick={() => { currentPath = '/' }}
    class="group relative mb-6 w-10 h-10 bg-white/[0.04] ring-1 ring-white/10 flex items-center justify-center transition-transform duration-300 ease-[cubic-bezier(0.32,0.72,0,1)] hover:scale-105"
    aria-label="{APP_NAME} — home"
  >
    <div class="w-5 h-5 [&_svg]:w-full [&_svg]:h-full">
      {@html logoMark}
    </div>
    <!-- SSE connection dot -->
    <span
      class="absolute -top-0.5 -right-0.5 w-2.5 h-2.5 rounded-full ring-2 ring-[var(--color-surface)] transition-colors duration-300 {sseState.connected ? 'bg-[var(--color-success)]' : 'bg-[var(--color-error)]'}"
      title={sseState.connected ? 'Connected to daemon' : 'Disconnected'}
    ></span>
  </a>

  <!-- Nav rail -->
  <nav class="flex-1 flex flex-col items-center gap-1.5" aria-label="Main navigation">
    {#each navItems as item}
      {@const active = isActive(item.href)}
      <Link
        to={item.href}
        onclick={() => { currentPath = item.href }}
      >
        <span
          class="group relative flex items-center justify-center w-11 h-11 transition-[background,color,transform] duration-300 ease-[cubic-bezier(0.32,0.72,0,1)] {active
            ? 'bg-[var(--accent-soft)] text-[var(--color-text)]'
            : 'text-[var(--color-muted)] hover:bg-white/5 hover:text-[var(--color-text)]'}"
        >
          <!-- active indicator pip -->
          <span
            class="absolute left-0 top-1/2 -translate-y-1/2 h-5 w-[3px] rounded-full bg-[var(--accent-text)] transition-all duration-300 ease-[cubic-bezier(0.32,0.72,0,1)] {active ? 'opacity-100' : 'opacity-0 -translate-x-1'}"
          ></span>

          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" class="shrink-0 transition-transform duration-300 ease-[cubic-bezier(0.32,0.72,0,1)] group-hover:scale-110">
            <path d={item.icon} stroke-linecap="round" stroke-linejoin="round" />
          </svg>

          <!-- hover tooltip -->
          <span
            class="pointer-events-none absolute left-[120%] top-1/2 -translate-y-1/2 translate-x-1 whitespace-nowrap bg-[var(--color-elevated)] px-2.5 py-1.5 text-xs font-medium text-[var(--color-text)] ring-1 ring-white/10 shadow-xl opacity-0 transition-all duration-200 ease-out group-hover:opacity-100 group-hover:translate-x-0 z-30"
          >
            {item.label}
          </span>
        </span>
      </Link>
    {/each}
  </nav>

  <!-- Bottom group: About + Sponsor -->
  <div class="flex flex-col items-center gap-1.5 pt-1.5">
    <Link to="/about" onclick={() => { currentPath = '/about' }}>
      <span
        class="group relative flex items-center justify-center w-11 h-11 transition-[background,color,transform] duration-300 ease-[cubic-bezier(0.32,0.72,0,1)] {isActive('/about')
          ? 'bg-[var(--accent-soft)] text-[var(--color-text)]'
          : 'text-[var(--color-muted)] hover:bg-white/5 hover:text-[var(--color-text)]'}"
      >
        <span
          class="absolute left-0 top-1/2 -translate-y-1/2 h-5 w-[3px] rounded-full bg-[var(--accent-text)] transition-all duration-300 ease-[cubic-bezier(0.32,0.72,0,1)] {isActive('/about') ? 'opacity-100' : 'opacity-0 -translate-x-1'}"
        ></span>

        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" class="shrink-0 transition-transform duration-300 ease-[cubic-bezier(0.32,0.72,0,1)] group-hover:scale-110">
          <circle cx="12" cy="12" r="10" />
          <path d="M12 16v-4M12 8h.01" stroke-linecap="round" stroke-linejoin="round" />
        </svg>

        <span
          class="pointer-events-none absolute left-[120%] top-1/2 -translate-y-1/2 translate-x-1 whitespace-nowrap bg-[var(--color-elevated)] px-2.5 py-1.5 text-xs font-medium text-[var(--color-text)] ring-1 ring-white/10 shadow-xl opacity-0 transition-all duration-200 ease-out group-hover:opacity-100 group-hover:translate-x-0 z-30"
        >
          About
        </span>
      </span>
    </Link>

    <a
      href="https://github.com/sponsors/modbender"
      target="_blank"
      rel="noopener"
      aria-label="Sponsor"
      class="group relative flex items-center justify-center w-11 h-11 text-[var(--color-muted)] transition-[background,color,transform] duration-300 ease-[cubic-bezier(0.32,0.72,0,1)] hover:bg-[#db61a2]/10 hover:text-[#db61a2]"
    >
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" class="shrink-0 transition-transform duration-300 ease-[cubic-bezier(0.32,0.72,0,1)] group-hover:scale-110">
        <path d="M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z" stroke-linecap="round" stroke-linejoin="round" />
      </svg>

      <span
        class="pointer-events-none absolute left-[120%] top-1/2 -translate-y-1/2 translate-x-1 whitespace-nowrap bg-[var(--color-elevated)] px-2.5 py-1.5 text-xs font-medium text-[var(--color-text)] ring-1 ring-white/10 shadow-xl opacity-0 transition-all duration-200 ease-out group-hover:opacity-100 group-hover:translate-x-0 z-30"
      >
        Sponsor
      </span>
    </a>
  </div>
</aside>
