import type { ComponentProps } from 'react'
import { cn } from '../cn'

/** Scroll region for wide tables; keeps headings sticky and focus visible inside the clip. */
function TableScroll({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div
      data-slot="table-scroll"
      role="region"
      tabIndex={0}
      className={cn('max-h-(--table-max-height) overflow-auto focus-ring-inset', className)}
      {...props}
    />
  )
}

function Table({ className, ...props }: ComponentProps<'table'>) {
  return (
    <table
      data-slot="table"
      className={cn('w-full border-separate border-spacing-0 text-left text-body', className)}
      {...props}
    />
  )
}

function TableHeader({ className, ...props }: ComponentProps<'thead'>) {
  return (
    <thead
      data-slot="table-header"
      className={cn('sticky top-0 z-(--layer-sticky)', className)}
      {...props}
    />
  )
}

function TableBody({ className, ...props }: ComponentProps<'tbody'>) {
  return (
    <tbody
      data-slot="table-body"
      className={cn('[&>tr:last-child>td]:border-b-0 [&>tr:hover]:bg-surface-raised', className)}
      {...props}
    />
  )
}

function TableRow({ className, ...props }: ComponentProps<'tr'>) {
  return <tr data-slot="table-row" className={className} {...props} />
}

const numericCell = 'text-right tabular-nums whitespace-nowrap'

function TableHead({
  className,
  numeric = false,
  ...props
}: ComponentProps<'th'> & { numeric?: boolean }) {
  return (
    <th
      data-slot="table-head"
      className={cn(
        'relative border-y border-line bg-surface-heading px-3 py-0 text-label font-medium whitespace-nowrap text-ink-muted aria-[sort=ascending]:text-selected-ink aria-[sort=descending]:text-selected-ink',
        numeric && numericCell,
        className,
      )}
      {...props}
    />
  )
}

function TableCell({
  className,
  numeric = false,
  ...props
}: ComponentProps<'td'> & { numeric?: boolean }) {
  return (
    <td
      data-slot="table-cell"
      className={cn(
        'h-(--table-row-height) border-b border-line px-3 py-1 align-middle',
        numeric && numericCell,
        className,
      )}
      {...props}
    />
  )
}

/** Column heading action: inherits heading ink, turns blue on hover. */
function TableSortButton({ className, type = 'button', ...props }: ComponentProps<'button'>) {
  return (
    <button
      data-slot="table-sort-button"
      type={type}
      className={cn(
        'inline-flex min-h-(--control-height-compact) items-center gap-1 rounded-sm border-0 bg-transparent py-1 focus-ring-inset hover:text-selected-ink [&_svg]:size-(--icon-size)',
        className,
      )}
      {...props}
    />
  )
}

/** Drag/keyboard handle on a column edge; the caller wires sizing behavior. */
function TableResizeHandle({
  className,
  resizing = false,
  ...props
}: ComponentProps<'div'> & { resizing?: boolean }) {
  return (
    <div
      data-slot="table-resize-handle"
      data-resizing={resizing || undefined}
      role="separator"
      tabIndex={0}
      aria-orientation="vertical"
      className={cn(
        'group absolute inset-y-0 end-0 w-3 cursor-col-resize touch-none select-none focus-ring-inset',
        'after:absolute after:inset-y-2 after:end-0 after:w-(--border-width) after:bg-line',
        'hover:after:w-0.5 hover:after:bg-selected-ink focus-visible:after:w-0.5 focus-visible:after:bg-selected-ink data-resizing:after:w-0.5 data-resizing:after:bg-selected-ink',
        className,
      )}
      {...props}
    />
  )
}

export {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableResizeHandle,
  TableRow,
  TableScroll,
  TableSortButton,
}
