import { cva } from 'class-variance-authority'
import type { VariantProps } from 'class-variance-authority'
import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

const alertVariants = cva('relative flex gap-3 rounded-lg border p-4 text-body', {
  variants: {
    variant: {
      default: 'border-line bg-surface text-ink',
      destructive: 'border-status-danger bg-status-danger-surface text-status-danger',
    },
  },
  defaultVariants: { variant: 'default' },
})

function Alert({
  className,
  variant,
  ...props
}: ComponentProps<'div'> & VariantProps<typeof alertVariants>) {
  return <div data-slot="alert" className={cn(alertVariants({ variant }), className)} {...props} />
}

function AlertTitle({ className, ...props }: ComponentProps<'div'>) {
  return <div data-slot="alert-title" className={cn('font-semibold', className)} {...props} />
}

function AlertDescription({ className, ...props }: ComponentProps<'div'>) {
  return <div data-slot="alert-description" className={cn('text-body', className)} {...props} />
}

export { Alert, AlertDescription, AlertTitle }
