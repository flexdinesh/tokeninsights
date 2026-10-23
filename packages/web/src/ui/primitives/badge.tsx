import { cva } from 'class-variance-authority'
import type { VariantProps } from 'class-variance-authority'
import type { ComponentProps } from 'react'
import { cn } from '../cn'

const badgeVariants = cva(
  'inline-flex w-fit shrink-0 items-center justify-center gap-1 rounded-sm border px-1.5 py-0.5 text-label font-medium tabular-nums',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-action text-action-ink',
        secondary: 'border-transparent bg-surface-raised text-ink-muted',
        outline: 'border-line text-ink',
        destructive: 'border-transparent bg-status-danger text-status-danger-ink',
      },
    },
    defaultVariants: { variant: 'secondary' },
  },
)

function Badge({
  className,
  variant,
  ...props
}: ComponentProps<'span'> & VariantProps<typeof badgeVariants>) {
  return <span data-slot="badge" className={cn(badgeVariants({ variant }), className)} {...props} />
}

export { Badge, badgeVariants }
