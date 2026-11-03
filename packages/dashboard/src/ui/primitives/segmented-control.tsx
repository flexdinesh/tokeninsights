import type { ReactNode } from 'react'
import { cn } from '../cn'
import { Button } from './button'

type Option<T extends string> = { value: T; label: ReactNode }

/** Exclusive choice rendered as pressed toggle buttons in a labelled group. */
function SegmentedControl<T extends string>({
  label,
  options,
  value,
  onValueChange,
  variant = 'quiet',
  size = 'compact',
  className,
}: {
  label: string
  options: Option<T>[]
  value: T | undefined
  onValueChange: (value: T) => void
  variant?: 'quiet' | 'outline'
  size?: 'compact' | 'default'
  className?: string
}) {
  return (
    <div
      role="group"
      aria-label={label}
      data-slot="segmented-control"
      className={cn('flex flex-wrap items-center gap-1', className)}
    >
      {options.map((option) => (
        <Button
          key={option.value}
          variant={variant}
          size={size}
          aria-pressed={option.value === value}
          onClick={() => onValueChange(option.value)}
        >
          {option.label}
        </Button>
      ))}
    </div>
  )
}

export { SegmentedControl }
