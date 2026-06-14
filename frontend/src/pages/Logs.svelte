<script lang="ts">
  import LogStream from '$lib/components/LogStream.svelte'
</script>

<div class="flex flex-col h-full">
  <!-- Page header -->
  <div class="sticky top-0 z-10 flex items-center justify-between px-6 sm:px-10 py-4 border-b border-[var(--color-border)] bg-[var(--color-bg)]/95 backdrop-blur-md">
    <h1 class="text-[15px] font-semibold tracking-tight">Logs</h1>
  </div>

  <LogStream />
</div>
