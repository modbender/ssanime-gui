<script lang="ts">
  import { cn } from '$lib/utils'

  type Variant = 'default' | 'secondary' | 'ghost' | 'destructive' | 'outline'
  type Size = 'sm' | 'md' | 'lg' | 'icon'

  let {
    variant = 'default',
    size = 'md',
    disabled = false,
    class: cls = '',
    onclick,
    type = 'button',
    title,
    children,
  }: {
    variant?: Variant
    size?: Size
    disabled?: boolean
    class?: string
    onclick?: (e: MouseEvent) => void
    type?: 'button' | 'submit' | 'reset'
    title?: string
    children?: any
  } = $props()

  const variantClasses: Record<Variant, string> = {
    default:
      'bg-[var(--accent)] text-white shadow-[0_4px_18px_-6px_rgb(var(--accent-rgb)/0.7)] hover:brightness-110',
    secondary:
      'bg-[var(--color-surface-2)] text-[var(--color-text)] border border-[var(--color-border)] hover:bg-[var(--color-surface-3)] hover:border-[var(--color-border-strong)]',
    ghost: 'text-[var(--color-text-dim)] hover:bg-white/5 hover:text-[var(--color-text)]',
    destructive: 'bg-[var(--color-error)] text-white hover:brightness-110',
    outline:
      'border border-[var(--color-border)] text-[var(--color-text-dim)] hover:bg-white/5 hover:text-[var(--color-text)] hover:border-[var(--color-border-strong)]',
  }

  const sizeClasses: Record<Size, string> = {
    sm: 'h-7 px-3 text-xs rounded-lg',
    md: 'h-9 px-4 text-[13px] rounded-xl',
    lg: 'h-11 px-6 text-sm rounded-xl',
    icon: 'h-9 w-9 rounded-xl flex items-center justify-center',
  }
</script>

<button
  {type}
  {disabled}
  {title}
  onclick={onclick}
  class={cn(
    'inline-flex items-center justify-center gap-1.5 font-medium cursor-pointer select-none',
    'transition-[background,transform,filter,border-color,box-shadow] duration-300 ease-[cubic-bezier(0.32,0.72,0,1)]',
    'active:scale-[0.97] disabled:opacity-45 disabled:cursor-not-allowed disabled:active:scale-100',
    variantClasses[variant],
    sizeClasses[size],
    cls,
  )}
>
  {@render children?.()}
</button>
