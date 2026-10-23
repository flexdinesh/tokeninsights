import type { ReactNode } from 'react'
import { cn } from '../cn'
import { Brand } from './brand'

/** Dashboard frame: sticky sidebar beside a header and scrolling content column. */
export function AppShell({
  sidebar,
  header,
  skipTarget,
  children,
}: {
  sidebar: ReactNode
  header: ReactNode
  /** Id of the main region the skip link jumps to. */
  skipTarget?: string
  children: ReactNode
}) {
  return (
    <div className="grid min-h-svh grid-cols-(--shell-columns) max-nav:grid-cols-1">
      {skipTarget && (
        <a
          href={`#${skipTarget}`}
          className="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-(--layer-skip-link) focus:bg-surface focus:p-2 focus:text-ink"
        >
          Skip to dashboard
        </a>
      )}
      {sidebar}
      <div className="min-w-0">
        {header}
        {children}
      </div>
    </div>
  )
}

/** Centered content column with the shared page gutter and width cap. */
export function PageContent({
  id,
  className,
  children,
}: {
  id?: string
  className?: string
  children: ReactNode
}) {
  return (
    <main
      id={id}
      className={cn(
        'mx-auto max-w-content px-(--page-gutter) pt-2 pb-3 max-compact:pb-4',
        className,
      )}
    >
      {children}
    </main>
  )
}

/** Brand header above a centered message or form: login, connection, route errors. */
export function StartupShell({
  role,
  className,
  children,
}: {
  role?: 'alert'
  className?: string
  children: ReactNode
}) {
  return (
    <div className="min-h-svh">
      <header className="flex min-h-(--header-height) items-center border-b border-line bg-surface px-(--page-gutter)">
        <Brand />
      </header>
      <StartupContent role={role} className={className}>
        {children}
      </StartupContent>
    </div>
  )
}

/** Centered startup message without its own header, for use inside another frame. */
export function StartupContent({
  role,
  className,
  children,
}: {
  role?: 'alert'
  className?: string
  children: ReactNode
}) {
  return (
    <main
      role={role}
      className={cn(
        'flex min-h-(--startup-min-height) flex-col items-center justify-center gap-4 px-(--page-gutter) py-8 text-center [&>p]:max-w-reading [&>svg]:size-6',
        className,
      )}
    >
      {children}
    </main>
  )
}
