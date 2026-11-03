import type { ReactNode } from 'react'
import { cn } from '../cn'

/** Rule-separated strip of static readouts; two columns in narrow containers. */
function ReadoutStrip({
  className,
  children,
  ...props
}: {
  className?: string
  children: ReactNode
  'aria-label': string
  'data-mounted'?: string
}) {
  return (
    <section
      data-slot="readout-strip"
      className={cn('@container/readouts mb-2 border-b border-line', className)}
      {...props}
    >
      <div className="grid grid-cols-(--readout-columns) gap-4 py-2 @max-readouts/readouts:grid-cols-(--readout-columns-narrow)">
        {children}
      </div>
    </section>
  )
}

/** One static measurement. The first readout of a strip spans both narrow columns. */
function Readout({
  label,
  value,
  valueLabel,
  detail,
  primary = false,
}: {
  label: ReactNode
  value: ReactNode
  /** Accessible full value, e.g. the exact count behind an abbreviated value. */
  valueLabel: string
  detail?: ReactNode
  primary?: boolean
}) {
  return (
    <div
      data-slot="readout"
      className={cn(
        'flex min-w-0 flex-col items-start gap-1 border-line text-left',
        primary
          ? '@max-readouts/readouts:col-span-full'
          : 'border-l pl-4 @max-readouts/readouts:border-l-0 @max-readouts/readouts:pl-0',
      )}
    >
      <span className="text-label text-ink-muted">{label}</span>
      <strong
        aria-label={valueLabel}
        className="flex min-h-(--readout-value-height) items-center text-readout leading-tight font-semibold tracking-tight tabular-nums"
      >
        {value}
      </strong>
      {detail && <span className="text-label wrap-anywhere text-ink-subtle">{detail}</span>}
    </div>
  )
}

export { Readout, ReadoutStrip }
