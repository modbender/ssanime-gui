// Promise-based confirm dialog. Replaces native confirm(), which renders as an
// ugly "127.0.0.1 says…" box and is unreliable inside the Tauri WebView2 shell.
//
// A single ConfirmHost is mounted once in App.svelte and drives this singleton
// state. Call sites: `if (!(await confirm({ title, message }))) return`.

export interface ConfirmOptions {
  title: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  destructive?: boolean
}

interface ConfirmDialog extends Required<ConfirmOptions> {
  open: boolean
  resolve: ((ok: boolean) => void) | null
}

export const confirmState = $state<ConfirmDialog>({
  open: false,
  title: '',
  message: '',
  confirmLabel: 'Confirm',
  cancelLabel: 'Cancel',
  destructive: false,
  resolve: null,
})

export function confirm(opts: ConfirmOptions): Promise<boolean> {
  // A second prompt while one is open resolves the first as cancelled, so a
  // dangling promise can't leak.
  if (confirmState.resolve) confirmState.resolve(false)

  return new Promise<boolean>((resolve) => {
    confirmState.title = opts.title
    confirmState.message = opts.message
    confirmState.confirmLabel = opts.confirmLabel ?? 'Confirm'
    confirmState.cancelLabel = opts.cancelLabel ?? 'Cancel'
    confirmState.destructive = opts.destructive ?? false
    confirmState.resolve = resolve
    confirmState.open = true
  })
}

export function resolveConfirm(ok: boolean) {
  confirmState.resolve?.(ok)
  confirmState.resolve = null
  confirmState.open = false
}
