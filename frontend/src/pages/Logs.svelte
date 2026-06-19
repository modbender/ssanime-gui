<script lang="ts">
  import LogStream from '$lib/components/LogStream.svelte'
  import { scrollScrim } from '$lib/scrollScrim'
</script>

<div class="flex flex-col h-full" use:scrollScrim>
  <!-- Page header -->
  <div class="sticky top-0 z-10 flex items-center justify-between px-6 sm:px-10 py-4 bg-transparent backdrop-blur-0 border-b border-transparent transition-[background-color,border-color,backdrop-filter] duration-300 [.scrolled_&]:bg-[var(--color-bg)]/85 [.scrolled_&]:backdrop-blur-md [.scrolled_&]:border-[var(--color-border)]">
    <h1 class="text-[15px] font-semibold tracking-tight">Logs</h1>
  </div>

  <LogStream />
</div>
