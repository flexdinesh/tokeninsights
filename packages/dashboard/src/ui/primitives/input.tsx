import type { ComponentProps } from 'react'
import { cn } from '../cn'

// Field typography grows on compact screens so mobile browsers do not zoom on focus.
const fieldText = 'text-body max-compact:text-title'

function Input({ className, type, ...props }: ComponentProps<'input'>) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        'min-h-(--control-height) min-w-0 rounded-md border border-line-strong bg-surface-field px-3 py-1 text-ink caret-action transition-colors duration-(--duration-fast) placeholder:text-ink-subtle focus-ring enabled:hover:bg-surface-raised disabled:pointer-events-none disabled:opacity-(--opacity-disabled) aria-invalid:border-status-danger',
        fieldText,
        className,
      )}
      {...props}
    />
  )
}

export { fieldText, Input }
