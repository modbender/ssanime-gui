// Installed-extension cache + the soft-gate signal for download actions.
//
// The app ships zero built-in sources; everything is a user-installed JS
// extension. Download/track stays enabled in the UI, but if no enabled source
// exists the action is intercepted by `requireSource()` and routed to the
// Extensions page instead of firing.

import { api, type Extension } from '$lib/api'

export const sourcesState = $state<{ extensions: Extension[]; loaded: boolean }>({
  extensions: [],
  loaded: false,
})

export async function reloadSources() {
  try {
    sourcesState.extensions = await api.listExtensions()
  } catch {
    sourcesState.extensions = []
  } finally {
    sourcesState.loaded = true
  }
}

/** True when at least one installed extension is enabled. */
export function hasEnabledSource(): boolean {
  return sourcesState.extensions.some((e) => e.enabled)
}

// ---- Soft-gate prompt ----
//
// A single "Add a source first" modal is mounted once in App.svelte. Call sites
// invoke `requireSource()` before any download/track action: it returns true to
// let the action proceed, or opens the prompt and returns false to bail.

export const gateState = $state<{ open: boolean }>({ open: false })

export function requireSource(): boolean {
  if (hasEnabledSource()) return true
  gateState.open = true
  return false
}

export function closeGate() {
  gateState.open = false
}
