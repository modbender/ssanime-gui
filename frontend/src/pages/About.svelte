<script lang="ts">
  import { api, type VersionInfo } from '$lib/api'
  import { APP_NAME } from '$lib/app'
  import logoMark from '$lib/assets/logo-mark.svg?raw'

  let version = $state<VersionInfo | null>(null)
  let versionLoading = $state(true)

  $effect(() => {
    let cancelled = false
    api
      .getVersion()
      .then((v) => { if (!cancelled) version = v })
      .catch(() => {}) // best-effort: omit the version line on failure
      .finally(() => { if (!cancelled) versionLoading = false })
    return () => { cancelled = true }
  })

  /** git-describe may already prefix "v"; strip one so we never render "vv". */
  const versionLabel = $derived(
    version ? `v${version.version.replace(/^v/, '')}` : '',
  )
  const commitShort = $derived(version ? version.commit.slice(0, 7) : '')

  const links = [
    {
      label: 'GitHub repository',
      href: 'https://github.com/modbender/ssanime-gui',
      icon: 'M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22',
    },
    {
      label: 'Report an issue',
      href: 'https://github.com/modbender/ssanime-gui/issues',
      icon: 'M12 9v4m0 4h.01M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z',
    },
    {
      label: 'License (GPL-3.0)',
      href: 'https://github.com/modbender/ssanime-gui/blob/main/LICENSE',
      icon: 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6Zm0 0v6h6M9 13h6m-6 4h6',
    },
  ]
</script>

<div class="flex h-full flex-col">
  <!-- Page header -->
  <div class="sticky top-0 z-10 flex items-center justify-between border-b border-[var(--color-border)] bg-[var(--color-bg)]/95 px-6 py-4 backdrop-blur-md sm:px-10">
    <h1 class="text-[15px] font-semibold tracking-tight">About</h1>
  </div>

  <!-- Body -->
  <div class="flex-1 overflow-y-auto px-6 py-10 sm:px-10">
    <div class="mx-auto max-w-xl animate-fade-up space-y-8">
      <!-- Identity -->
      <div class="flex flex-col items-center text-center">
        <div class="flex h-16 w-16 items-center justify-center bg-white/[0.04] ring-1 ring-white/10">
          <div class="h-8 w-8 [&_svg]:h-full [&_svg]:w-full">
            {@html logoMark}
          </div>
        </div>
        <h2 class="mt-4 text-2xl font-extrabold tracking-tight text-[var(--color-text)]">{APP_NAME}</h2>
        <p class="mt-1.5 text-sm text-[var(--color-text-dim)]">
          Local, UI-first anime download → encode → archive manager.
        </p>

        <!-- Version -->
        <div class="mt-4 h-6">
          {#if versionLoading}
            <div class="h-4 w-28 animate-pulse bg-[var(--color-surface-3)]"></div>
          {:else if version}
            <span class="inline-flex items-center gap-2 bg-[var(--color-surface-2)] px-3 py-1 font-mono text-xs text-[var(--color-text-dim)] ring-1 ring-[var(--color-border)]">
              <span class="tabular-nums">{versionLabel}</span>
              {#if commitShort}
                <span class="text-[var(--color-faint)]">·</span>
                <span class="tabular-nums">{commitShort}</span>
              {/if}
            </span>
          {/if}
        </div>
      </div>

      <!-- Description -->
      <p class="text-center text-sm leading-relaxed text-[var(--color-text-dim)]">
        SSAnime downloads anime from torrents and streaming sources, re-encodes every
        episode with ffmpeg into smaller permanent x265 files, and manages the resulting
        local library — auto-fetching new episodes as they air. It is free and
        open-source software, licensed under the
        <a
          href="https://github.com/modbender/ssanime-gui/blob/main/LICENSE"
          target="_blank"
          rel="noopener"
          class="text-[var(--color-text)] underline decoration-[var(--color-border-strong)] underline-offset-2 transition-colors hover:decoration-[var(--accent)]"
        >GPL-3.0</a> license.
      </p>

      <!-- Link row -->
      <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
        {#each links as link}
          <a
            href={link.href}
            target="_blank"
            rel="noopener"
            class="group flex items-center justify-center gap-2 border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 text-[13px] font-medium text-[var(--color-text-dim)] transition-colors duration-200 hover:border-[var(--color-border-strong)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" class="shrink-0">
              <path d={link.icon} stroke-linecap="round" stroke-linejoin="round" />
            </svg>
            {link.label}
          </a>
        {/each}
      </div>

      <!-- Sponsor -->
      <div class="border border-[var(--color-border)] bg-[var(--color-surface)] p-5 text-center">
        <p class="text-sm font-medium text-[var(--color-text)]">Support development</p>
        <p class="mt-1 text-[13px] text-[var(--color-muted)]">
          SSAnime is built in the open. If it saves you time, consider sponsoring its
          continued development.
        </p>
        <a
          href="https://github.com/sponsors/modbender"
          target="_blank"
          rel="noopener"
          class="mt-4 inline-flex h-10 items-center justify-center gap-2 bg-[#db61a2] px-5 text-[13px] font-semibold text-white transition-[filter,transform] duration-200 hover:brightness-110 active:scale-[0.97]"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <path d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35Z" />
          </svg>
          Sponsor on GitHub
        </a>
      </div>
    </div>
  </div>
</div>
