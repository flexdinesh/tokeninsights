import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

function Input({ className, type, ...props }: ComponentProps<'input'>) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        'min-h-(--control-height) min-w-0 rounded-md border border-line-strong bg-surface px-3 py-1 text-body outline-none transition-colors placeholder:text-ink-subtle hover:bg-surface-hover disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-status-danger focus-visible:border-focus focus-visible:ring-2 focus-visible:ring-focus/30',
        className,
      )}
      {...props}
    />
  )
}

export { Input }
