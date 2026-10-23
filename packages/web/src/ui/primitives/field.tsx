import { useId } from 'react'
import type { ReactNode } from 'react'
import { cn } from '../cn'

type FieldControlProps = {
  id: string
  'aria-describedby': string | undefined
  'aria-invalid': true | undefined
}

/**
 * Label, control, help text and error as one unit. The render prop receives the
 * ids and validity attributes the control must carry.
 */
function Field({
  label,
  description,
  error,
  className,
  children,
}: {
  label: ReactNode
  description?: ReactNode
  error?: ReactNode
  className?: string
  children: (control: FieldControlProps) => ReactNode
}) {
  const id = useId()
  const descriptionId = description ? `${id}-description` : undefined
  const errorId = error ? `${id}-error` : undefined
  const describedBy = [descriptionId, errorId].filter(Boolean).join(' ') || undefined
  return (
    <div data-slot="field" className={cn('grid gap-1', className)}>
      <label htmlFor={id} className="text-body font-medium">
        {label}
      </label>
      {children({ id, 'aria-describedby': describedBy, 'aria-invalid': error ? true : undefined })}
      {description && (
        <p id={descriptionId} className="text-label text-ink-subtle">
          {description}
        </p>
      )}
      {error && (
        <p id={errorId} role="alert" className="text-label text-status-danger">
          {error}
        </p>
      )}
    </div>
  )
}

export { Field }
export type { FieldControlProps }
