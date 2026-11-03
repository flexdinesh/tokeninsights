import * as CheckboxPrimitive from '@radix-ui/react-checkbox'
import { Check } from 'lucide-react'
import type { ComponentProps, ReactNode } from 'react'
import { cn } from '../cn'

function Checkbox({ className, ...props }: ComponentProps<typeof CheckboxPrimitive.Root>) {
  return (
    <CheckboxPrimitive.Root
      data-slot="checkbox"
      className={cn(
        'peer size-(--icon-size) shrink-0 rounded-sm border border-line-strong bg-surface transition-colors duration-(--duration-fast) focus-ring disabled:pointer-events-none disabled:opacity-(--opacity-disabled) data-[state=checked]:border-action data-[state=checked]:bg-action data-[state=checked]:text-action-ink',
        className,
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator
        data-slot="checkbox-indicator"
        className="grid place-content-center text-current [&_svg]:size-3.5"
      >
        <Check />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  )
}

/** Checkbox with its label as one compact, hoverable row. */
function CheckboxOption({
  id,
  label,
  title,
  ...props
}: ComponentProps<typeof CheckboxPrimitive.Root> & {
  id: string
  label: ReactNode
  title?: string
}) {
  return (
    <label
      htmlFor={id}
      data-slot="checkbox-option"
      className="flex min-h-(--control-height-compact) cursor-pointer items-center gap-2 rounded-sm px-1 text-body hover:bg-surface-raised"
    >
      <Checkbox id={id} {...props} />
      <span className="wrap-anywhere" title={title}>
        {label}
      </span>
    </label>
  )
}

export { Checkbox, CheckboxOption }
