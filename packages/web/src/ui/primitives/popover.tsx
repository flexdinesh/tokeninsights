import * as PopoverPrimitive from '@radix-ui/react-popover'
import type { ComponentProps, ReactNode } from 'react'
import { cn } from '../cn'

function Popover(props: ComponentProps<typeof PopoverPrimitive.Root>) {
  return <PopoverPrimitive.Root data-slot="popover" {...props} />
}

function PopoverTrigger(props: ComponentProps<typeof PopoverPrimitive.Trigger>) {
  return <PopoverPrimitive.Trigger data-slot="popover-trigger" {...props} />
}

function PopoverContent({
  className,
  align = 'center',
  sideOffset = 8,
  collisionPadding = 16,
  ...props
}: ComponentProps<typeof PopoverPrimitive.Content>) {
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Content
        data-slot="popover-content"
        align={align}
        sideOffset={sideOffset}
        collisionPadding={collisionPadding}
        className={cn(
          'z-(--layer-popover) max-h-(--radix-popover-content-available-height) w-(--popover-width) overflow-y-auto rounded-lg border border-line bg-surface-overlay p-3 text-ink shadow-overlay outline-none data-[state=closed]:animate-out data-[state=open]:animate-in data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95',
          className,
        )}
        {...props}
      />
    </PopoverPrimitive.Portal>
  )
}

/** Popover title row with an optional trailing action such as Clear. */
function PopoverHeader({
  title,
  action,
  className,
}: {
  title: ReactNode
  action?: ReactNode
  className?: string
}) {
  return (
    <div
      data-slot="popover-header"
      className={cn(
        'flex min-h-(--control-height-compact) items-center justify-between gap-2',
        className,
      )}
    >
      <strong className="text-body">{title}</strong>
      {action}
    </div>
  )
}

function PopoverClose(props: ComponentProps<typeof PopoverPrimitive.Close>) {
  return <PopoverPrimitive.Close data-slot="popover-close" {...props} />
}

export { Popover, PopoverClose, PopoverContent, PopoverHeader, PopoverTrigger }
