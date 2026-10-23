import { useId, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  ChevronLeft,
  ChevronRight,
  ChevronDown,
  Columns3,
  Copy,
} from 'lucide-react'
import {
  columnResizingFeature,
  columnSizingFeature,
  columnVisibilityFeature,
  flexRender,
  tableFeatures,
  useTable,
} from '@tanstack/react-table'
import type { ColumnDef } from '@tanstack/react-table'
import type { Dashboard, Row, Sort, Tab } from '../contracts'
import { sortSchema } from '../contracts'
import {
  exactCount,
  formatCount,
  formatServerDate,
  formatServerDateTime,
  labels,
  serverTimeZoneLabel,
} from '../format'
import { useDashboardPreferences, useDashboardQuery } from '../state'
import {
  Button,
  CheckboxOption,
  cn,
  ControlLabel,
  EmptyState,
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTrigger,
  SectionHeader,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableResizeHandle,
  TableRow,
  TableScroll,
  TableSortButton,
} from '@/ui'

interface Column {
  id: Sort
  label: string
  numeric?: boolean
}
const resizeStep = 16
// Sorting and pagination are server-owned; the table only sizes and hides columns.
const features = tableFeatures({
  columnVisibilityFeature,
  columnSizingFeature,
  columnResizingFeature,
})
const minColumnSize = 112
const maxColumnSize = 480

const tokenColumns: Column[] = [
  { id: 'total', label: 'Total', numeric: true },
  { id: 'input', label: 'Input', numeric: true },
  { id: 'output', label: 'Output', numeric: true },
  { id: 'cacheRead', label: 'Cache R', numeric: true },
  { id: 'cacheWrite', label: 'Cache W', numeric: true },
  { id: 'reasoning', label: 'Reasoning', numeric: true },
]

function columnsFor(tab: Tab): Column[] {
  if (tab === 'context')
    return [
      { id: 'harness', label: 'Harness' },
      { id: 'provider', label: 'Provider' },
      { id: 'model', label: 'Model' },
      { id: 'sessions', label: 'Sessions', numeric: true },
      { id: 'averageContext', label: 'Avg ctx', numeric: true },
      { id: 'medianContext', label: 'Median ctx', numeric: true },
      { id: 'maxContext', label: 'Max ctx', numeric: true },
    ]
  if (tab === 'repo') {
    return [
      { id: 'name', label: 'Location' },
      { id: 'provider', label: 'Providers' },
      { id: 'harness', label: 'Harnesses' },
      { id: 'model', label: 'Models' },
      { id: 'sessions', label: 'Sessions', numeric: true },
      ...tokenColumns,
    ]
  }
  const first: Column = {
    id: 'name',
    label:
      tab === 'tokens'
        ? 'Period'
        : tab === 'sessions'
          ? 'Session'
          : tab === 'harnesses'
            ? 'Harness'
            : labels[tab].slice(0, -1),
  }
  return [
    first,
    ...(tab === 'sessions'
      ? [{ id: 'date', label: 'Last active' } satisfies Column]
      : [{ id: 'sessions', label: 'Sessions', numeric: true } satisfies Column]),
    ...tokenColumns,
    ...(tab === 'sessions'
      ? [{ id: 'context', label: 'Ctx used', numeric: true } satisfies Column]
      : []),
  ]
}

function summaryLines(value: string): string[] {
  return value
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean)
}

function identityDetails(row: Row, tab: Tab): string[] {
  if (tab === 'tokens') return []
  if (tab === 'repo')
    return row.repositoryName !== 'unknown' && row.repositoryName !== row.locationName
      ? [row.repositoryName]
      : []
  return [
    ...new Set(
      [
        tab !== 'harnesses' ? row.harness : '',
        tab !== 'providers' ? row.provider : '',
        tab !== 'models' ? row.model : '',
      ].filter(Boolean),
    ),
  ].flatMap(summaryLines)
}

const identityCell = 'flex min-w-0 max-w-full flex-col gap-1'
const identityName = 'truncate font-medium'
const identityDetail = 'flex flex-col text-label wrap-anywhere text-ink-subtle empty:hidden'
const dimensionValue = 'inline-block min-w-0 max-w-full wrap-anywhere'

export function UnknownLocation({
  directories,
  hasUnknownDirectory,
}: {
  directories: string[]
  hasUnknownDirectory: boolean
}) {
  const [expanded, setExpanded] = useState(false)
  const listId = useId()

  return (
    <div className={identityCell}>
      <div className="flex flex-wrap items-center gap-x-2">
        <span className={identityName}>unknown</span>
        {directories.length > 0 && (
          <Button
            variant="link"
            size="compact"
            className="gap-1"
            aria-expanded={expanded}
            aria-controls={listId}
            onClick={() => setExpanded((value) => !value)}
          >
            {expanded ? 'Hide directories' : 'Show directories'}
            <ChevronDown
              aria-hidden="true"
              className={cn(
                'transition-transform duration-(--duration-disclosure) ease-out',
                expanded && 'rotate-180',
              )}
            />
          </Button>
        )}
      </div>
      {directories.length > 0 && (
        <ul
          className="grid gap-1 border-s border-line ps-2"
          id={listId}
          aria-label="Recorded directories"
          hidden={!expanded}
        >
          {directories.map((directory) => (
            <li
              key={directory}
              className="font-mono text-label wrap-anywhere text-ink-subtle"
              title={directory}
            >
              {directory}
            </li>
          ))}
        </ul>
      )}
      {hasUnknownDirectory && (expanded || directories.length === 0) && (
        <span className={identityDetail}>
          {directories.length > 0
            ? 'Some usage has no recorded directory'
            : 'No recorded directory'}
        </span>
      )}
    </div>
  )
}

type DisplayRow = Row

function ResultsCell({
  row,
  spec,
  tab,
  timezone,
}: {
  row: DisplayRow
  spec: Column
  tab: Tab
  timezone: string
}): ReactNode {
  const value = row[spec.id]
  if (spec.id === 'date')
    return (
      <time
        title={`${formatServerDateTime(row.date, timezone)} · ${serverTimeZoneLabel(timezone)}`}
        dateTime={new Date(row.date).toISOString()}
      >
        {formatServerDate(row.date, timezone)}
      </time>
    )
  if (spec.numeric && typeof value === 'number')
    return <span title={exactCount(value)}>{formatCount(value)}</span>
  if (spec.id === 'name' && tab === 'repo' && row.locationKey === 'unknown')
    return (
      <UnknownLocation
        directories={row.directoryNames}
        hasUnknownDirectory={row.hasUnknownDirectory}
      />
    )
  if (spec.id === 'name') {
    const details = identityDetails(row, tab)
    return (
      <div className={identityCell}>
        <div className="flex min-w-0 items-center gap-2">
          <span
            className={cn(
              identityName,
              'min-w-0',
              tab === 'tokens' && 'overflow-visible whitespace-normal wrap-anywhere tabular-nums',
            )}
            title={String(value)}
          >
            {String(value)}
          </span>
        </div>
        <span className={identityDetail}>
          {details.map((detail) => (
            <span key={detail}>{detail}</span>
          ))}
        </span>
      </div>
    )
  }
  if (spec.id === 'harness' || spec.id === 'provider' || spec.id === 'model')
    return (
      <span className={dimensionValue}>
        {summaryLines(String(value)).map((line) => (
          <span key={line} className="block">
            {line}
          </span>
        ))}
      </span>
    )
  return (
    <span className={dimensionValue} title={String(value)}>
      {String(value)}
    </span>
  )
}

export function ResultsTable({
  data,
  timezone,
  hasExcludedUsage,
}: {
  data: Dashboard
  timezone: string
  hasExcludedUsage: boolean
}) {
  const { query, sortBy, setPage, setPageSize, clearFilters, setQuality } = useDashboardQuery()
  const reviewingExcluded = query.quality === 'estimated'
  const baseTitle = query.tab === 'repo' ? 'Repo totals' : `${labels[query.tab]} breakdown`
  const title = reviewingExcluded ? `Excluded ${baseTitle.toLowerCase()}` : baseTitle
  const hasFilters =
    [
      query.providers,
      query.models,
      query.harnesses,
      query.sessions,
      query.repositories,
      query.directories,
    ].some((values) => values.length > 0) || Boolean(query.from || query.to)
  const { hiddenColumns: hidden, toggleColumn } = useDashboardPreferences()
  const specs = useMemo(() => columnsFor(query.tab), [query.tab])
  const displayRows = data.rows
  const columnId = useId()
  const columns = useMemo<ColumnDef<typeof features, DisplayRow>[]>(
    () =>
      specs.map((spec) => ({
        id: spec.id,
        accessorFn: (row) => row[spec.id],
        header: spec.label,
        size: spec.id === 'name' ? (query.tab === 'repo' ? 256 : 224) : spec.numeric ? 128 : 160,
        minSize: spec.id === 'name' ? 176 : spec.numeric ? minColumnSize : 128,
        maxSize: maxColumnSize,
      })),
    [specs, query.tab],
  )
  const visibility = Object.fromEntries(
    specs.map((s) => [s.id, !hidden.includes(s.id) || s.id === 'name']),
  )
  const table = useTable({
    features,
    data: displayRows,
    columns,
    columnResizeMode: 'onChange',
    getRowId: (row) => row.key,
    state: { columnVisibility: visibility },
  })
  const pages = Math.max(1, Math.ceil(data.rowCount / data.pageSize))
  const sortOptions: Column[] =
    query.tab === 'context'
      ? specs
      : [
          { id: 'date', label: 'Date' },
          { id: 'name', label: 'Name' },
          ...tokenColumns,
          { id: 'sessions', label: 'Sessions' },
          { id: 'context', label: 'Ctx used' },
        ]
  return (
    <section
      className="overflow-hidden border-t border-line"
      aria-label={`${reviewingExcluded ? 'Excluded ' : ''}${labels[query.tab]} details`}
    >
      <SectionHeader
        title={title}
        count={exactCount(data.rowCount)}
        actions={
          <>
            <ControlLabel label="Sort">
              <Select
                value={query.sort}
                onValueChange={(value) => {
                  const sort = sortSchema.safeParse(value)
                  if (sort.success) sortBy(sort.data)
                }}
              >
                <SelectTrigger size="compact" aria-label="Sort by">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {sortOptions.map((s) => (
                    <SelectItem key={s.id} value={s.id}>
                      {s.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </ControlLabel>
            <Button
              variant="outline"
              size="icon-compact"
              aria-label={`Sort ${query.direction === 'asc' ? 'descending' : 'ascending'}`}
              onClick={() => sortBy(query.sort)}
            >
              {query.direction === 'asc' ? (
                <ArrowUp aria-hidden="true" />
              ) : (
                <ArrowDown aria-hidden="true" />
              )}
            </Button>
            <Popover>
              <PopoverTrigger asChild>
                <Button variant="outline" size="compact">
                  <Columns3 aria-hidden="true" />
                  Columns
                </Button>
              </PopoverTrigger>
              <PopoverContent align="end" aria-label="Visible columns">
                <PopoverHeader title="Visible columns" />
                {specs
                  .filter((s) => s.id !== 'name')
                  .map((s) => (
                    <CheckboxOption
                      key={s.id}
                      id={`${columnId}-${s.id}`}
                      label={s.label}
                      checked={!hidden.includes(s.id)}
                      onCheckedChange={() => toggleColumn(s.id)}
                    />
                  ))}
              </PopoverContent>
            </Popover>
          </>
        }
      />
      <TableScroll aria-label="Scrollable results">
        <Table className="table-fixed" style={{ width: table.getTotalSize(), minWidth: '100%' }}>
          <colgroup>
            {table.getVisibleLeafColumns().map((column) => (
              <col key={column.id} style={{ width: column.getSize() }} />
            ))}
          </colgroup>
          <TableHeader>
            {table.getHeaderGroups().map((group) => (
              <TableRow key={group.id}>
                {group.headers.map((header) => {
                  const spec = specs.find((s) => s.id === header.id)
                  if (!spec) return null
                  const sort = spec.id === 'name' && query.tab === 'tokens' ? 'date' : spec.id
                  const active = query.sort === sort
                  return (
                    <TableHead
                      key={header.id}
                      numeric={spec.numeric}
                      aria-label={spec.label}
                      aria-sort={
                        active ? (query.direction === 'asc' ? 'ascending' : 'descending') : 'none'
                      }
                    >
                      <TableSortButton onClick={() => sortBy(sort)}>
                        {flexRender(header.column.columnDef.header, header.getContext())}
                        {active ? (
                          query.direction === 'asc' ? (
                            <ArrowUp aria-hidden="true" />
                          ) : (
                            <ArrowDown aria-hidden="true" />
                          )
                        ) : (
                          <ArrowUpDown aria-hidden="true" className="text-ink-subtle" />
                        )}
                      </TableSortButton>
                      <TableResizeHandle
                        resizing={header.column.getIsResizing()}
                        aria-label={`Resize ${spec.label} column`}
                        aria-valuemin={header.column.columnDef.minSize}
                        aria-valuemax={header.column.columnDef.maxSize}
                        aria-valuenow={header.column.getSize()}
                        title="Drag to resize; double-click to reset"
                        onMouseDown={header.getResizeHandler()}
                        onTouchStart={header.getResizeHandler()}
                        onDoubleClick={() => header.column.resetSize()}
                        onKeyDown={(event) => {
                          if (event.key === 'Home') {
                            event.preventDefault()
                            header.column.resetSize()
                          } else if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
                            event.preventDefault()
                            const direction = event.key === 'ArrowRight' ? 1 : -1
                            const size = Math.min(
                              maxColumnSize,
                              Math.max(
                                header.column.columnDef.minSize ?? minColumnSize,
                                header.column.getSize() + direction * resizeStep,
                              ),
                            )
                            table.setColumnSizing((current) => ({
                              ...current,
                              [header.column.id]: size,
                            }))
                          }
                        }}
                      />
                    </TableHead>
                  )
                })}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows.map((row) => (
              <TableRow key={row.id}>
                {row.getVisibleCells().map((cell) => {
                  const spec = specs.find((candidate) => candidate.id === cell.column.id)
                  if (!spec) return null
                  return (
                    <TableCell key={cell.id} numeric={spec.numeric}>
                      <ResultsCell
                        row={row.original}
                        spec={spec}
                        tab={query.tab}
                        timezone={timezone}
                      />
                    </TableCell>
                  )
                })}
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {data.rowCount === 0 && displayRows.length === 0 && (
          <EmptyState
            icon={<Copy aria-hidden="true" />}
            title={
              reviewingExcluded
                ? 'No excluded usage matches these filters'
                : 'No usage matches these filters'
            }
            description={
              <>
                {reviewingExcluded
                  ? 'Only usage excluded from your total appears here.'
                  : 'Your date range and filters apply to usage included in your total.'}{' '}
                Try a wider date range{hasFilters ? ' or clear your filters' : ''}.
              </>
            }
            actions={
              (hasFilters || hasExcludedUsage || reviewingExcluded) && (
                <>
                  {hasFilters && (
                    <Button variant="outline" onClick={clearFilters}>
                      Clear filters
                    </Button>
                  )}
                  {(hasExcludedUsage || reviewingExcluded) && (
                    <Button
                      variant="ghost"
                      onClick={() => setQuality(reviewingExcluded ? 'confirmed' : 'estimated')}
                    >
                      {reviewingExcluded ? 'Back to usage' : 'Review excluded usage'}
                    </Button>
                  )}
                </>
              )
            }
          />
        )}
      </TableScroll>
      <footer className="flex flex-wrap items-center justify-between gap-4 border-t border-line py-1 text-label text-ink-subtle [&_strong]:text-ink [&_strong]:tabular-nums">
        <div className="flex min-w-0 flex-wrap items-baseline gap-y-1">
          <span
            className="wrap-anywhere"
            title={
              reviewingExcluded
                ? 'Shown: sessions with excluded usage matching all filters, across all pages. All dates: sessions with excluded usage in this dataset, ignoring filters.'
                : 'Shown: distinct countable sessions matching all filters, across all pages. Synced: all countable sessions in this dataset, ignoring filters.'
            }
          >
            {reviewingExcluded ? 'Excluded sessions' : 'Sessions'}{' '}
            <strong>{exactCount(data.summary.sessions)}</strong> shown /{' '}
            <strong>{exactCount(data.summary.syncedSessions)}</strong>{' '}
            {reviewingExcluded ? 'across all dates' : 'synced'}
          </span>
          <SummaryDot />
          <span>
            <strong>{exactCount(data.rowCount)}</strong> rows
          </span>
          {query.tab !== 'context' && (
            <>
              <SummaryDot />
              <span>
                {reviewingExcluded ? 'Excluded total' : 'Total'}{' '}
                <strong title={exactCount(data.summary.total)}>
                  {formatCount(data.summary.total)}
                </strong>
              </span>
            </>
          )}
        </div>
        <div className="ml-auto flex items-center gap-2">
          <div className="mr-2 max-compact:hidden">
            <ControlLabel label="Rows">
              <Select
                value={String(query.pageSize)}
                onValueChange={(value) => setPageSize(Number(value))}
              >
                <SelectTrigger size="compact" aria-label="Rows per page">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {[25, 50, 100, 200].map((n) => (
                    <SelectItem key={n} value={String(n)}>
                      {n}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </ControlLabel>
          </div>
          <span className="tabular-nums">
            {data.page} / {pages}
          </span>
          <Button
            variant="outline"
            size="icon"
            disabled={data.page <= 1}
            onClick={() => setPage(data.page - 1)}
            aria-label="Previous page"
          >
            <ChevronLeft aria-hidden="true" />
          </Button>
          <Button
            variant="outline"
            size="icon"
            disabled={data.page >= pages}
            onClick={() => setPage(data.page + 1)}
            aria-label="Next page"
          >
            <ChevronRight aria-hidden="true" />
          </Button>
        </div>
      </footer>
    </section>
  )
}

function SummaryDot() {
  return (
    <span aria-hidden="true" className="mx-2">
      ·
    </span>
  )
}
