import type { ReactNode } from 'react'
import { Button } from '../primitives/button'
import { Brand } from './brand'

/**
 * Desktop navigation rail; below the nav breakpoint it becomes a wrapping top
 * region. Brand, links and footer share one inline start edge.
 */
export function Sidebar({
  label,
  navLabel,
  footer,
  children,
}: {
  label: string
  navLabel: string
  footer?: ReactNode
  children: ReactNode
}) {
  return (
    <aside
      aria-label={label}
      className="sticky top-0 flex h-svh min-w-0 flex-col gap-2 self-start overflow-y-auto border-r border-line bg-surface-rail px-3 pb-4 max-nav:relative max-nav:h-auto max-nav:gap-1 max-nav:overflow-visible max-nav:border-r-0 max-nav:border-b max-nav:px-(--page-gutter) max-nav:pb-2"
    >
      <div className="flex min-h-(--header-height) items-center px-3 max-nav:px-0">
        <Brand />
      </div>
      <nav aria-label={navLabel} className="grid min-w-0 gap-1 max-nav:flex max-nav:flex-wrap">
        {children}
      </nav>
      {footer && (
        <div className="mt-auto grid gap-3 px-3 text-label text-ink-subtle max-nav:mt-0 max-nav:px-0 max-nav:empty:hidden max-nav:[&>p]:hidden">
          {footer}
        </div>
      )}
    </aside>
  )
}

/** Navigation link; pass a router link as the child. Current page uses aria-current. */
export function SidebarItem({ children }: { children: ReactNode }) {
  return (
    <Button
      variant="quiet"
      asChild
      className="justify-start focus-ring-inset max-nav:flex-none max-compact:gap-1 max-compact:px-1 max-compact:text-label"
    >
      {children}
    </Button>
  )
}
