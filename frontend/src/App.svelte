<script lang="ts">
  import { Router, Route } from 'svelte-routing'
  import Sidebar from '$lib/components/Sidebar.svelte'
  import Home from './pages/Home.svelte'
  import Library from './pages/Library.svelte'
  import SeriesDetail from './pages/SeriesDetail.svelte'
  import Activity from './pages/Activity.svelte'
  import Profiles from './pages/Profiles.svelte'
  import Settings from './pages/Settings.svelte'
  import Logs from './pages/Logs.svelte'
  import About from './pages/About.svelte'
  import Extensions from './pages/Extensions.svelte'
  import WelcomeModal from '$lib/components/WelcomeModal.svelte'
  import SourceGateModal from '$lib/components/SourceGateModal.svelte'
  import { startSSE } from '$lib/sse.svelte'
  import { reloadSources } from '$lib/sources.svelte'
  import { activityState, startActivity } from '$lib/activity.svelte'
  import { overallPercent } from '$lib/pipeline.svelte'
  import { setOverallProgress, clearOverallProgress } from '$lib/taskbar'

  $effect(() => {
    const stop = startSSE()
    return stop
  })

  // Load the installed-source signal once on app start; download gating reads it.
  $effect(() => {
    reloadSources()
  })

  // Prime the active-episode set and keep it fresh against SSE status churn.
  $effect(() => startActivity())

  // Drive the OS taskbar / favicon-ring from the mean progress of active jobs.
  $effect(() => {
    setOverallProgress(overallPercent(activityState.activeEpisodes))
  })
  // Restore the static favicon/title + clear the taskbar once, on app teardown.
  $effect(() => clearOverallProgress)
</script>

<Router>
  <div class="flex h-screen bg-[var(--color-bg)] text-[var(--color-text)] overflow-hidden">
    <Sidebar />
    <main class="flex-1 overflow-hidden flex flex-col min-w-0">
      <Route path="/">
        <Home />
      </Route>
      <Route path="/library">
        <Library />
      </Route>
      <Route path="/series/anilist/:anilistId" let:params>
        <SeriesDetail anilistId={params.anilistId} />
      </Route>
      <Route path="/series/:id" let:params>
        <SeriesDetail id={params.id} />
      </Route>
      <Route path="/activity">
        <Activity />
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
      <Route path="/extensions">
        <Extensions />
      </Route>
      <Route path="/about">
        <About />
      </Route>
    </main>
  </div>

  <WelcomeModal />
  <SourceGateModal />
</Router>
