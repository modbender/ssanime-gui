// App-wide toast notifications. Replaces native alert(), which renders as an
// ugly "127.0.0.1 says…" box and is unreliable inside the Tauri WebView2 shell.
//
// A single ToastHost is mounted once in App.svelte and renders this stack. Call
// sites use toast.error/success/info; each toast auto-dismisses and is manually
// dismissible.

export type ToastKind = 'error' | 'success' | 'info'

export interface Toast {
  id: number
  kind: ToastKind
  message: string
}

// Errors linger longer than the rest — they're the ones worth reading.
const TIMEOUT: Record<ToastKind, number> = {
  error: 7000,
  success: 3500,
  info: 4000,
}

export const toastState = $state<{ toasts: Toast[] }>({ toasts: [] })

let nextId = 1
const timers = new Map<number, ReturnType<typeof setTimeout>>()

export function dismissToast(id: number) {
  toastState.toasts = toastState.toasts.filter((t) => t.id !== id)
  const timer = timers.get(id)
  if (timer) {
    clearTimeout(timer)
    timers.delete(id)
  }
}

function show({ kind, message }: { kind: ToastKind; message: string }): number {
  const id = nextId++
  toastState.toasts = [...toastState.toasts, { id, kind, message }]
  timers.set(
    id,
    setTimeout(() => dismissToast(id), TIMEOUT[kind]),
  )
  return id
}

export const toast = {
  show,
  error: (message: string) => show({ kind: 'error', message }),
  success: (message: string) => show({ kind: 'success', message }),
  info: (message: string) => show({ kind: 'info', message }),
}
