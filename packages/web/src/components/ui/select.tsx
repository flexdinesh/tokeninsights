import * as SelectPrimitive from '@radix-ui/react-select'
import { cva } from 'class-variance-authority'
import type { VariantProps } from 'class-variance-authority'
import { Check, ChevronDown, ChevronUp } from 'lucide-react'
import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

const selectTriggerVariants = cva(
  'flex items-center justify-between gap-2 rounded-md border border-line-strong bg-surface py-1 outline-none transition-colors hover:bg-surface-hover disabled:pointer-events-none disabled:opacity-50 focus-visible:border-focus focus-visible:ring-2 focus-visible:ring-focus/30 [&_svg]:size-4 [&_svg]:shrink-0',
  {
    variants: {
      size: {
        sm: 'min-h-(--control-height-compact) px-2.5 text-label',
        default: 'min-h-(--control-height) px-3 text-body',
        lg: 'min-h-(--control-height-lg) px-4 text-title',
      },
    },
    defaultVariants: { size: 'default' },
  },
)

function Select(props: ComponentProps<typeof SelectPrimitive.Root>) {
  return <SelectPrimitive.Root data-slot="select" {...props} />
}

function SelectValue(props: ComponentProps<typeof SelectPrimitive.Value>) {
  return <SelectPrimitive.Value data-slot="select-value" {...props} />
}

function SelectTrigger({
  className,
  children,
  size,
  ...props
}: ComponentProps<typeof SelectPrimitive.Trigger> & VariantProps<typeof selectTriggerVariants>) {
  return (
    <SelectPrimitive.Trigger
      data-slot="select-trigger"
      className={cn(selectTriggerVariants({ size }), className)}
      {...props}
    >
      {children}
      <SelectPrimitive.Icon asChild>
        <ChevronDown className="text-ink-subtle" />
      </SelectPrimitive.Icon>
    </SelectPrimitive.Trigger>
  )
}

function SelectContent({
  className,
  children,
  position = 'popper',
  ...props
}: ComponentProps<typeof SelectPrimitive.Content>) {
  return (
    <SelectPrimitive.Portal>
      <SelectPrimitive.Content
        data-slot="select-content"
        position={position}
        className={cn(
          'z-(--layer-popover) max-h-(--radix-select-content-available-height) min-w-32 overflow-y-auto rounded-lg border border-line bg-surface-overlay text-ink shadow-overlay data-[state=closed]:animate-out data-[state=open]:animate-in data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95',
          position === 'popper' &&
            'data-[side=bottom]:translate-y-1 data-[side=left]:-translate-x-1 data-[side=right]:translate-x-1 data-[side=top]:-translate-y-1',
          className,
        )}
        {...props}
      >
        <SelectPrimitive.ScrollUpButton className="flex h-6 items-center justify-center">
          <ChevronUp className="size-4" />
        </SelectPrimitive.ScrollUpButton>
        <SelectPrimitive.Viewport className="p-1">{children}</SelectPrimitive.Viewport>
        <SelectPrimitive.ScrollDownButton className="flex h-6 items-center justify-center">
          <ChevronDown className="size-4" />
        </SelectPrimitive.ScrollDownButton>
      </SelectPrimitive.Content>
    </SelectPrimitive.Portal>
  )
}

function SelectItem({
  className,
  children,
  ...props
}: ComponentProps<typeof SelectPrimitive.Item>) {
  return (
    <SelectPrimitive.Item
      data-slot="select-item"
      className={cn(
        'relative flex min-h-(--control-height-compact) cursor-default select-none items-center rounded-sm py-1 pl-7 pr-2 text-body outline-none data-[disabled]:pointer-events-none data-[highlighted]:bg-surface-hover data-[highlighted]:text-ink-hover data-[disabled]:opacity-50',
        className,
      )}
      {...props}
    >
      <span className="absolute left-2 grid size-4 place-content-center">
        <SelectPrimitive.ItemIndicator>
          <Check className="size-4" />
        </SelectPrimitive.ItemIndicator>
      </span>
      <SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
    </SelectPrimitive.Item>
  )
}

export { Select, SelectContent, SelectItem, SelectTrigger, SelectValue }
