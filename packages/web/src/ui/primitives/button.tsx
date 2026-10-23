import { Slot } from '@radix-ui/react-slot'
import { cva } from 'class-variance-authority'
import type { VariantProps } from 'class-variance-authority'
import type { ComponentProps } from 'react'
import { cn } from '../cn'

const buttonVariants = cva(
  'inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-md border border-transparent font-medium transition-colors duration-(--duration-fast) focus-ring disabled:pointer-events-none disabled:opacity-(--opacity-disabled) [&_svg]:pointer-events-none [&_svg]:size-(--icon-size) [&_svg]:shrink-0 selected:border-transparent selected:bg-selected selected:text-selected-ink',
  {
    variants: {
      variant: {
        default: 'bg-action text-action-ink hover:bg-action/90',
        outline: 'border-line-strong bg-surface hover:bg-surface-hover hover:text-ink-hover',
        secondary: 'bg-surface-raised text-ink-muted hover:bg-surface-raised/80',
        ghost: 'hover:bg-surface-hover hover:text-ink-hover',
        quiet: 'text-ink-muted hover:bg-surface-hover hover:text-ink-hover',
        link: 'text-ink-subtle underline-offset-(--underline-offset) hover:text-ink hover:underline',
        destructive: 'bg-status-danger text-status-danger-ink hover:bg-status-danger/90',
      },
      size: {
        default: 'min-h-(--control-height) px-3 py-1 text-body',
        compact: 'min-h-(--control-height-compact) rounded-sm px-2.5 text-label',
        lg: 'min-h-(--control-height-lg) px-4 text-title',
        icon: 'size-(--control-height) p-0',
        'icon-compact': 'size-(--control-height-compact) rounded-sm p-0',
        'icon-lg': 'size-(--control-height-lg) p-0',
      },
    },
    compoundVariants: [{ variant: 'link', className: 'px-0' }],
    defaultVariants: { variant: 'default', size: 'default' },
  },
)

type ButtonProps = ComponentProps<'button'> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean
    /** Marks a non-toggle control as holding an active selection. */
    selected?: boolean
  }

function Button({
  className,
  variant,
  size,
  asChild = false,
  selected,
  type,
  ...props
}: ButtonProps) {
  const Comp = asChild ? Slot : 'button'
  return (
    <Comp
      data-slot="button"
      data-variant={variant ?? 'default'}
      data-selected={selected || undefined}
      type={asChild ? type : (type ?? 'button')}
      className={cn(buttonVariants({ variant, size }), className)}
      {...props}
    />
  )
}

export { Button, buttonVariants }
export type { ButtonProps }
