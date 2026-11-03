import type { ReactNode } from 'react'
import { Heading } from './typography'

/** Explains an empty result and offers recovery actions. */
function EmptyState({
  icon,
  title,
  description,
  actions,
}: {
  icon?: ReactNode
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
}) {
  return (
    <div
      data-slot="empty-state"
      className="flex flex-col items-center px-4 py-12 text-center text-ink-subtle [&>svg]:size-6"
    >
      {icon}
      <Heading as="h3" variant="sub" className="mt-3 mb-1 text-ink">
        {title}
      </Heading>
      {description && <p className="max-w-prose text-body text-ink-muted">{description}</p>}
      {actions && (
        <div className="mt-3 flex flex-wrap items-center justify-center gap-x-4 gap-y-2">
          {actions}
        </div>
      )}
    </div>
  )
}

export { EmptyState }
