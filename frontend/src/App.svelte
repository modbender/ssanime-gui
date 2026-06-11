<script lang="ts">
  import { Router, Route } from 'svelte-routing'
  import Sidebar from '$lib/components/Sidebar.svelte'
  import Home from './pages/Home.svelte'
  import Downloads from './pages/Downloads.svelte'
  import SeriesDetail from './pages/SeriesDetail.svelte'
  import Queue from './pages/Queue.svelte'
  import Feeds from './pages/Feeds.svelte'
  import Profiles from './pages/Profiles.svelte'
  import Settings from './pages/Settings.svelte'
  import Logs from './pages/Logs.svelte'
  import { startSSE } from '$lib/sse.svelte'

  $effect(() => {
    const stop = startSSE()
    return stop
  })
</script>

<Router>
  <div class="flex h-screen bg-[var(--color-bg)] text-[var(--color-text)] overflow-hidden">
    <Sidebar />
    <main class="flex-1 overflow-hidden flex flex-col min-w-0">
      <Route path="/">
        <Home />
      </Route>
      <Route path="/downloads">
        <Downloads />
      </Route>
      <Route path="/series/anilist/:anilistId" let:params>
        <SeriesDetail anilistId={params.anilistId} />
      </Route>
      <Route path="/series/:id" let:params>
        <SeriesDetail id={params.id} />
      </Route>
      <Route path="/queue">
        <Queue />
      </Route>
      <Route path="/feeds">
        <Feeds />
      </Route>
      <Route path="/profiles">
        <Profiles />
      </Route>
      <Route path="/settings">
        <Settings />
      </Route>
      <Route path="/logs">
        <Logs />
      </Route>
    </main>
  </div>
</Router>
