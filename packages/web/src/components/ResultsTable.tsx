import { useId, useMemo } from 'react'
import type { ReactNode } from 'react'
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  ChevronLeft,
  ChevronRight,
  Columns3,
  Copy,
} from 'lucide-react'
import { flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table'
import type { ColumnDef } from '@tanstack/react-table'
import type { Dashboard, Row, Sort, Tab } from '../contracts'
import { sortSchema } from '../contracts'
import { exactCount, formatCount, labels } from '../format'
import { useDashboardState } from '../state'
import { Badge } from './ui/badge'
import { Button } from './ui/button'
import { Checkbox } from './ui/checkbox'
import { Card } from './ui/card'
import { Popover, PopoverContent, PopoverTrigger } from './ui/popover'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from './ui/table'

interface Column {
  id: Sort
  label: string
  numeric?: boolean
}
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
  const first: Column = {
    id: 'name',
    label: tab === 'tokens' ? 'Period' : tab === 'sessions' ? 'Session' : labels[tab].slice(0, -1),
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

function identityDetail(row: Row, tab: Tab): string {
  if (tab === 'tokens') return ''
  return [
    ...new Set(
      [
        tab !== 'harnesses' ? row.harness : '',
        tab !== 'providers' ? row.provider : '',
        tab !== 'models' ? row.model : '',
      ].filter(Boolean),
    ),
  ].join(' · ')
}

function renderCell(row: Row, spec: Column, tab: Tab): ReactNode {
  const value = row[spec.id]
  if (spec.id === 'date')
    return (
      <time title={new Date(row.date).toISOString()} dateTime={new Date(row.date).toISOString()}>
        {new Date(row.date).toLocaleDateString()}
      </time>
    )
  if (spec.numeric && typeof value === 'number')
    return (
      <span className="numeric-value" title={exactCount(value)}>
        {formatCount(value)}
      </span>
    )
  if (spec.id === 'name')
    return (
      <div className="identity-cell">
        <span className="identity-name" title={String(value)}>
          {String(value)}
        </span>
        <span className="identity-detail" title={identityDetail(row, tab)}>
          {identityDetail(row, tab)}
        </span>
      </div>
    )
  return (
    <span className="dimension-value" title={String(value)}>
      {String(value)}
    </span>
  )
}

export function ResultsTable({ data }: { data: Dashboard }) {
  const {
    state: { query, hidden },
    dispatch,
  } = useDashboardState()
  const specs = useMemo(() => columnsFor(query.tab), [query.tab])
  const columnId = useId()
  const columns = useMemo<ColumnDef<Row>[]>(
    () =>
      specs.map((spec) => ({
        id: spec.id,
        accessorFn: (row) => row[spec.id],
        header: spec.label,
        cell: ({ row }) => renderCell(row.original, spec, query.tab),
      })),
    [specs, query.tab],
  )
  const visibility = Object.fromEntries(
    specs.map((s) => [s.id, !hidden.includes(s.id) || s.id === 'name']),
  )
  // oxlint-disable-next-line react/incompatible-library -- TanStack Table intentionally owns its memoized model.
  const table = useReactTable({
    data: data.rows,
    columns,
    getCoreRowModel: getCoreRowModel(),
    manualSorting: true,
    manualPagination: true,
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
    <Card className="table-panel panel" role="region" aria-label={`${labels[query.tab]} details`}>
      <div className="panel-heading">
        <div className="table-title">
          <h2>{labels[query.tab]} breakdown</h2>
          <Badge>{exactCount(data.rowCount)}</Badge>
        </div>
        <div className="table-controls">
          <div className="bucket-control">
            Sort
            <Select
              value={query.sort}
              onValueChange={(value) => {
                const sort = sortSchema.safeParse(value)
                if (sort.success) dispatch({ type: 'sort', value: sort.data })
              }}
            >
              <SelectTrigger aria-label="Sort by">
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
          </div>
          <Button
            variant="outline"
            size="icon"
            aria-label={`Sort ${query.direction === 'asc' ? 'descending' : 'ascending'}`}
            onClick={() => dispatch({ type: 'sort', value: query.sort })}
          >
            {query.direction === 'asc' ? <ArrowUp size="1em" /> : <ArrowDown size="1em" />}
          </Button>
          <Popover>
            <PopoverTrigger asChild>
              <Button variant="outline">
                <Columns3 size="1em" />
                Columns
              </Button>
            </PopoverTrigger>
            <PopoverContent align="end" aria-label="Visible columns">
              <strong>Visible columns</strong>
              {specs
                .filter((s) => s.id !== 'name')
                .map((s) => (
                  <label key={s.id} className="check-option" htmlFor={`${columnId}-${s.id}`}>
                    <Checkbox
                      id={`${columnId}-${s.id}`}
                      checked={!hidden.includes(s.id)}
                      onCheckedChange={() => dispatch({ type: 'column', value: s.id })}
                    />
                    {s.label}
                  </label>
                ))}
            </PopoverContent>
          </Popover>
        </div>
      </div>
      <div className="table-scroll" role="region" tabIndex={0} aria-label="Scrollable results">
        <Table>
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
                      className={spec.numeric ? 'numeric' : ''}
                      aria-sort={
                        active ? (query.direction === 'asc' ? 'ascending' : 'descending') : 'none'
                      }
                    >
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => dispatch({ type: 'sort', value: sort })}
                      >
                        {flexRender(header.column.columnDef.header, header.getContext())}
                        {active ? (
                          query.direction === 'asc' ? (
                            <ArrowUp size="0.9em" />
                          ) : (
                            <ArrowDown size="0.9em" />
                          )
                        ) : (
                          <ArrowUpDown size="0.9em" className="sort-hint" />
                        )}
                      </Button>
                    </TableHead>
                  )
                })}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows.map((row) => (
              <TableRow key={row.id}>
                {row.getVisibleCells().map((cell) => (
                  <TableCell
                    key={cell.id}
                    className={specs.find((s) => s.id === cell.column.id)?.numeric ? 'numeric' : ''}
                  >
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {data.rowCount === 0 && (
          <div className="empty-state">
            <SearchEmpty />
            <h3>No matching usage</h3>
            <p>Try a wider date range, clear filters, or sync your local data.</p>
          </div>
        )}
      </div>
      <footer className="results-summary">
        <div className="result-counts">
          <span
            className="session-coverage"
            title="Shown: distinct countable sessions matching all filters, across all pages. Synced: all countable sessions in this database, ignoring filters."
          >
            Sessions <strong>{exactCount(data.summary.sessions)}</strong> shown /{' '}
            <strong>{exactCount(data.summary.syncedSessions)}</strong> synced
          </span>
          <span className="summary-dot">·</span>
          <span>
            <strong>{exactCount(data.rowCount)}</strong> rows
          </span>
          {query.tab !== 'context' && (
            <>
              <span className="summary-dot">·</span>
              <span>
                Total{' '}
                <strong title={exactCount(data.summary.total)}>
                  {formatCount(data.summary.total)}
                </strong>
              </span>
            </>
          )}
        </div>
        <div className="pagination">
          <div className="pagination-size">
            Rows
            <Select
              value={String(query.pageSize)}
              onValueChange={(value) => dispatch({ type: 'pageSize', value: Number(value) })}
            >
              <SelectTrigger aria-label="Rows per page">
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
          </div>
          <span>
            {data.page} / {pages}
          </span>
          <Button
            variant="outline"
            size="icon"
            disabled={data.page <= 1}
            onClick={() => dispatch({ type: 'page', value: data.page - 1 })}
            aria-label="Previous page"
          >
            <ChevronLeft size="1em" />
          </Button>
          <Button
            variant="outline"
            size="icon"
            disabled={data.page >= pages}
            onClick={() => dispatch({ type: 'page', value: data.page + 1 })}
            aria-label="Next page"
          >
            <ChevronRight size="1em" />
          </Button>
        </div>
      </footer>
    </Card>
  )
}

function SearchEmpty() {
  return <Copy size="1.6em" className="muted" aria-hidden />
}
