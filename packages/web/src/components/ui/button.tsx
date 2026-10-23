import { Slot } from '@radix-ui/react-slot'
import { cva } from 'class-variance-authority'
import type { VariantProps } from 'class-variance-authority'
import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

const buttonVariants = cva(
  'inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-md border border-transparent text-body font-medium transition-colors outline-none disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0 focus-visible:ring-2 focus-visible:ring-focus focus-visible:ring-offset-2 focus-visible:ring-offset-surface',
  {
    variants: {
      variant: {
        default: 'bg-action text-action-ink hover:bg-action/90',
        outline: 'border-line-strong bg-surface hover:bg-surface-hover hover:text-ink-hover',
        secondary: 'bg-surface-raised text-ink-muted hover:bg-surface-raised/80',
        ghost: 'hover:bg-surface-hover hover:text-ink-hover',
        destructive: 'bg-status-danger text-status-danger-ink hover:bg-status-danger/90',
      },
      size: {
        default: 'min-h-(--control-height) px-3 py-1',
        sm: 'min-h-(--control-height-compact) rounded-sm px-2.5 text-label',
        lg: 'min-h-(--control-height-lg) px-4 text-title',
        icon: 'size-(--control-height) p-0',
        'icon-sm': 'size-(--control-height-compact) rounded-sm p-0',
        'icon-lg': 'size-(--control-height-lg) p-0',
      },
    },
    defaultVariants: { variant: 'default', size: 'default' },
  },
)

type ButtonProps = ComponentProps<'button'> &
  VariantProps<typeof buttonVariants> & { asChild?: boolean }

function Button({ className, variant, size, asChild = false, ...props }: ButtonProps) {
  const Comp = asChild ? Slot : 'button'
  return (
    <Comp
      data-slot="button"
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }
