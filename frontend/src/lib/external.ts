import { isTauri } from './taskbar'

// External links must round-trip through the OS in the Tauri shell: the WebView2
// window swallows `target="_blank"` and `window.open`, so a plain anchor silently
// does nothing. The opener plugin hands the URL to the system default browser.
// In a normal browser tab there's no plugin — fall back to a new tab.

/** Open an external URL in the system browser (Tauri) or a new tab (browser). */
export async function openExternal(url: string): Promise<void> {
  if (isTauri()) {
    try {
      const { openUrl } = await import('@tauri-apps/plugin-opener')
      await openUrl(url)
      return
    } catch {
      // plugin unavailable — fall through to the browser path
    }
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}

/**
 * Anchor click handler. In a plain browser the native anchor already works, so
 * this is a no-op there; inside Tauri it intercepts the click and routes the URL
 * through the OS. Attach as `onclick={(e) => externalClick(e, href)}`.
 */
export function externalClick(e: MouseEvent, url: string): void {
  if (!isTauri()) return
  e.preventDefault()
  void openExternal(url)
}
