import type { ReactNode } from 'react'
import { cn } from '../cn'
import { Badge } from './badge'
import { Heading } from './typography'

/** Title row for an analytics or settings section, with optional count and actions. */
function SectionHeader({
  title,
  description,
  hideDescription = false,
  count,
  actions,
  className,
}: {
  title: ReactNode
  description?: ReactNode
  /** Keep the description for assistive technology only. */
  hideDescription?: boolean
  count?: ReactNode
  actions?: ReactNode
  className?: string
}) {
  return (
    <div
      data-slot="section-header"
      className={cn(
        'flex flex-wrap items-center justify-between gap-x-4 gap-y-2 py-2 max-compact:items-start',
        className,
      )}
    >
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <Heading as="h2" variant="section">
            {title}
          </Heading>
          {count !== undefined && <Badge>{count}</Badge>}
        </div>
        {description && (
          <p className={hideDescription ? 'sr-only' : 'mt-1 text-label text-ink-subtle'}>
            {description}
          </p>
        )}
      </div>
      {actions && (
        <div className="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-x-3 gap-y-2 max-compact:ml-0 max-compact:justify-start">
          {actions}
        </div>
      )}
    </div>
  )
}

/** Label beside a compact control, e.g. "Sort" or "Bucket". */
function ControlLabel({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div data-slot="control-label" className="flex items-center gap-2 text-label text-ink-subtle">
      <span>{label}</span>
      {children}
    </div>
  )
}

export { ControlLabel, SectionHeader }
