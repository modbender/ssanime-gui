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
    default: 'bg-[#7c6af0] hover:bg-[#6a58e0] text-white',
    secondary: 'bg-[#18181f] hover:bg-[#2a2a35] text-[#e8e8f0] border border-[#2a2a35]',
    ghost: 'hover:bg-[#18181f] text-[#e8e8f0]',
    destructive: 'bg-red-600 hover:bg-red-700 text-white',
    outline: 'border border-[#2a2a35] hover:bg-[#18181f] text-[#e8e8f0]',
  }

  const sizeClasses: Record<Size, string> = {
    sm: 'h-7 px-2.5 text-xs rounded-md',
    md: 'h-9 px-4 text-sm rounded-lg',
    lg: 'h-10 px-5 text-sm rounded-lg',
    icon: 'h-8 w-8 rounded-md flex items-center justify-center',
  }
</script>

<button
  {type}
  {disabled}
  {title}
  onclick={onclick}
  class={cn(
    'inline-flex items-center gap-1.5 font-medium transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed focus-visible:outline-2 focus-visible:outline-[#7c6af0]',
    variantClasses[variant],
    sizeClasses[size],
    cls,
  )}
>
  {@render children?.()}
</button>
