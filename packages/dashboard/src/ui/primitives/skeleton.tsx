import type { ComponentProps } from 'react'
import { cn } from '../cn'

/** Placeholder block; callers size it with layout tokens such as h-(--chart-height). */
function Skeleton({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div
      data-slot="skeleton"
      className={cn('animate-pulse rounded-lg border border-line bg-surface-raised', className)}
      {...props}
    />
  )
}

export { Skeleton }
