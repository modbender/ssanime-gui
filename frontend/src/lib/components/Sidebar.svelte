<script lang="ts">
  import { Link } from 'svelte-routing'
  import { sseState } from '$lib/sse.svelte'
  import logoMark from '$lib/assets/logo-mark.svg?raw'

  const navItems = [
    { href: '/', label: 'Library', icon: 'M4 6a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6zm10 0a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2h-2a2 2 0 0 1-2-2V6zM4 16a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2v-2zm10 0a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2h-2a2 2 0 0 1-2-2v-2z' },
    { href: '/queue', label: 'Queue', icon: 'M4 6h16M4 10h16M4 14h16M4 18h16' },
    { href: '/feeds', label: 'Auto-downloader', icon: 'M15 17h5l-1.405-1.405A2.032 2.032 0 0 1 18 14.158V11a6.002 6.002 0 0 0-4-5.659V5a2 2 0 1 0-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 1 1-6 0v-1m6 0H9' },
    { href: '/profiles', label: 'Profiles', icon: 'M9 3H5a2 2 0 0 0-2 2v4m6-6h10a2 2 0 0 1 2 2v4M9 3v18m0 0h10a2 2 0 0 0 2-2V9M9 21H5a2 2 0 0 1-2-2V9m0 0h18' },
    { href: '/settings', label: 'Settings', icon: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 0 0 2.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 0 0 1.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 0 0-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 0 0-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 0 0-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 0 0-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 0 0 1.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065zM15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0z' },
    { href: '/logs', label: 'Logs', icon: 'M9 12h6m-6 4h6m2 5H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5.586a1 1 0 0 1 .707.293l5.414 5.414a1 1 0 0 1 .293.707V19a2 2 0 0 1-2 2z' },
  ]

  // Active path detection — use window.location since svelte-routing doesn't expose a reactive store easily
  let currentPath = $state(window.location.pathname)

  function isActive(href: string) {
    if (href === '/') return currentPath === '/'
    return currentPath.startsWith(href)
  }

  // Update on navigation
  $effect(() => {
    const handler = () => { currentPath = window.location.pathname }
    window.addEventListener('popstate', handler)
    return () => window.removeEventListener('popstate', handler)
  })
</script>

<aside class="flex flex-col h-full w-56 bg-[#0d0d14] border-r border-[#2a2a35] shrink-0">
  <!-- Logo -->
  <div class="flex items-center gap-2.5 px-4 h-14 border-b border-[#2a2a35]">
    <!-- Mark: three converging chevrons = compression / small size animations -->
    <div class="w-7 h-7 shrink-0 flex items-center justify-center">
      {@html logoMark}
    </div>
    <span class="text-[#e8e8f0] font-semibold text-sm tracking-tight">ssanime</span>

    <!-- SSE connection dot -->
    <div
      class="ml-auto w-2 h-2 rounded-full shrink-0 {sseState.connected ? 'bg-green-400' : 'bg-red-400'}"
      title={sseState.connected ? 'Connected' : 'Disconnected'}
    ></div>
  </div>

  <!-- Nav -->
  <nav class="flex-1 py-3 px-2 flex flex-col gap-0.5" aria-label="Main navigation">
    {#each navItems as item}
      <Link
        to={item.href}
        class="flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm transition-colors group {isActive(item.href) ? 'bg-[#7c6af0]/15 text-[#e8e8f0]' : 'text-[#6b6b80] hover:bg-[#18181f] hover:text-[#e8e8f0]'}"
        onclick={() => { currentPath = item.href }}
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" class="shrink-0">
          <path d={item.icon} stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        {item.label}
      </Link>
    {/each}
  </nav>

  <!-- Version footer -->
  <div class="px-4 py-3 border-t border-[#2a2a35] text-xs text-[#6b6b80]">
    ssanime-gui v0.1
  </div>
</aside>
