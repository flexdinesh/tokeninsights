import type { ReactNode } from 'react'
import { Heading } from '../primitives/typography'

/** Sticky header naming the current view, with trailing identity and actions. */
export function PageHeader({ title, children }: { title: ReactNode; children?: ReactNode }) {
  return (
    <header className="sticky top-0 z-(--layer-header) flex min-h-(--header-height) flex-wrap items-center justify-between gap-4 border-b border-line bg-surface px-(--page-gutter) py-1 max-compact:relative max-compact:gap-2">
      <Heading as="h1" variant="view">
        {title}
      </Heading>
      {children && (
        <div className="flex max-w-full min-w-0 flex-wrap items-center justify-end gap-2 max-compact:ml-auto max-compact:flex-1 max-compact:flex-nowrap max-compact:gap-1">
          {children}
        </div>
      )}
    </header>
  )
}

/** Thin vertical rule between header items; hidden once the header wraps. */
export function HeaderDivider() {
  return <span aria-hidden="true" className="mx-2 h-4 w-(--border-width) bg-line max-nav:hidden" />
}
