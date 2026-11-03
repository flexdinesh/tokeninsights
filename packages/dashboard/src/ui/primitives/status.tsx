import { Activity, CircleAlert, LoaderCircle } from 'lucide-react'
import type { ComponentProps } from 'react'
import { cn } from '../cn'

type StatusState = 'ready' | 'loading' | 'attention'

const stateInk: Record<StatusState, string> = {
  ready: 'text-ink-subtle',
  loading: 'text-selected-ink',
  attention: 'text-status-danger',
}

/** Rotating loader; rotation stops under reduced motion. */
function Spinner({ className, ...props }: ComponentProps<typeof LoaderCircle>) {
  return <LoaderCircle aria-hidden="true" className={cn('animate-spin', className)} {...props} />
}

/** Fixed-size icon for ready, loading and attention states. Pair with a text label. */
function StatusIcon({ state }: { state: StatusState }) {
  const className = cn('size-(--icon-size)', stateInk[state])
  if (state === 'loading') return <Spinner className={className} />
  if (state === 'attention') return <CircleAlert aria-hidden="true" className={className} />
  return <Activity aria-hidden="true" className={className} />
}

const dotTone = {
  success: 'bg-status-success',
  pending: 'bg-ink-subtle',
  danger: 'bg-status-danger',
}

/** Small circular status mark. Decorative; the adjacent text carries meaning. */
function StatusDot({ tone = 'success' }: { tone?: keyof typeof dotTone }) {
  return (
    <span
      aria-hidden="true"
      data-slot="status-dot"
      className={cn('inline-block size-2 shrink-0 rounded-full', dotTone[tone])}
    />
  )
}

export { Spinner, StatusDot, StatusIcon }
export type { StatusState }
