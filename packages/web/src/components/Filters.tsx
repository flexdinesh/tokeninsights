import { useEffect, useId, useRef, useState } from 'react'
import * as Popover from '@radix-ui/react-popover'
import { CalendarDays, Check, ChevronDown, Filter, RotateCcw, Search, X } from 'lucide-react'
import type { Dimension, Facets, Selection } from '../contracts'
import { bucketSchema, periodSchema } from '../contracts'
import { useDashboardState } from '../state'
import { useFacets } from '../api'

const dimensions: { key: Dimension; label: string }[] = [
  { key: 'harnesses', label: 'Harness' },
  { key: 'providers', label: 'Provider' },
  { key: 'models', label: 'Model' },
  { key: 'sessions', label: 'Session' },
]
const periods: { value: Selection['period']; label: string }[] = [
  { value: 'today', label: 'Today' },
  { value: 'yesterday', label: 'Yesterday' },
  { value: 'week', label: 'This week' },
  { value: 'month', label: 'This month' },
  { value: 'year', label: 'This year' },
  { value: 'all', label: 'All time' },
]

export function MultiSelect({
  label,
  values,
  selected,
  onChange,
  onSearch,
  loading = false,
}: {
  label: string
  values: string[]
  selected: string[]
  onChange: (values: string[]) => void
  onSearch?: (search: string) => void
  loading?: boolean
}) {
  const [search, setSearch] = useState('')
  const searchInput = useRef<HTMLInputElement>(null)
  const options = [...new Set([...selected, ...values])]
    // oxlint-disable-next-line unicorn/no-array-sort -- This array is freshly created.
    .sort()
    .filter((v) => v.toLowerCase().includes(search.toLowerCase()))
  return (
    <Popover.Root>
      <Popover.Trigger className={`filter-button ${selected.length ? 'is-selected' : ''}`}>
        {label}
        {selected.length > 0 && <span className="count-badge">{selected.length}</span>}
        <ChevronDown size="1em" />
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          className="popover"
          sideOffset={8}
          collisionPadding={16}
          align="start"
          aria-label={`${label} filter`}
          onOpenAutoFocus={(event) => {
            event.preventDefault()
            searchInput.current?.focus()
          }}
        >
          <div className="popover-heading">
            <strong>{label}</strong>
            <button className="text-button" onClick={() => onChange([])}>
              Clear
            </button>
          </div>
          <label className="search-field">
            <Search size="1em" />
            <input
              ref={searchInput}
              aria-label={`Search ${label.toLowerCase()}`}
              placeholder={`Find ${label.toLowerCase()}…`}
              value={search}
              onChange={(e) => {
                setSearch(e.target.value)
                onSearch?.(e.target.value)
              }}
            />
          </label>
          <div className="filter-options" aria-busy={loading}>
            {options.map((value) => (
              <label className="check-option" key={value}>
                <input
                  type="checkbox"
                  checked={selected.includes(value)}
                  onChange={() =>
                    onChange(
                      selected.includes(value)
                        ? selected.filter((v) => v !== value)
                        : [...selected, value],
                    )
                  }
                />
                <span title={value}>{value}</span>
              </label>
            ))}
            {options.length === 0 && (
              <p className="muted">{loading ? 'Loading…' : 'No matching values'}</p>
            )}
          </div>
          {onSearch && <p className="hint">First 100 matches. Search to narrow results.</p>}
          <Popover.Close className="button primary full-width">
            <Check size="1em" />
            Done
          </Popover.Close>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}

function DateFilter() {
  const {
    state: { query },
    dispatch,
  } = useDashboardState()
  const errorId = useId()
  const [open, setOpen] = useState(false)
  const [from, setFrom] = useState(query.from)
  const [to, setTo] = useState(query.to)
  const invalid = Boolean(from && to && from > to)
  const custom = query.from || query.to
  return (
    <Popover.Root
      open={open}
      onOpenChange={(value) => {
        setOpen(value)
        if (value) {
          setFrom(query.from)
          setTo(query.to)
        }
      }}
    >
      <Popover.Trigger className="filter-button date-button">
        <CalendarDays size="1em" />
        {custom
          ? `${query.from || 'Beginning'} → ${query.to || 'Now'}`
          : periods.find((p) => p.value === query.period)?.label}
        <ChevronDown size="1em" />
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          className="popover date-popover"
          align="end"
          sideOffset={8}
          collisionPadding={16}
          aria-label="Date range"
        >
          <strong>Date range</strong>
          <div className="date-presets">
            {periods.map((p) => (
              <button
                key={p.value}
                className={`button ${!custom && query.period === p.value ? 'selected' : ''}`}
                onClick={() => {
                  dispatch({ type: 'selection', value: { period: p.value, from: '', to: '' } })
                  setOpen(false)
                }}
              >
                {p.label}
              </button>
            ))}
          </div>
          <div className="date-inputs">
            <label>
              From
              <input
                aria-label="From date"
                aria-invalid={invalid || undefined}
                aria-describedby={invalid ? errorId : undefined}
                type="date"
                value={from}
                onChange={(e) => setFrom(e.target.value)}
              />
            </label>
            <label>
              To
              <input
                aria-label="To date"
                aria-invalid={invalid || undefined}
                aria-describedby={invalid ? errorId : undefined}
                type="date"
                value={to}
                onChange={(e) => setTo(e.target.value)}
              />
            </label>
          </div>
          <p className="hint">
            Inclusive dates in the server’s local timezone. Either bound can be empty.
          </p>
          {invalid && (
            <p id={errorId} role="alert" className="error-text">
              From must not be after to.
            </p>
          )}
          <button
            className="button primary full-width"
            disabled={invalid || (!from && !to)}
            onClick={() => {
              dispatch({ type: 'selection', value: { from, to } })
              setOpen(false)
            }}
          >
            Apply range
          </button>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}

export function FilterToolbar({
  baseUrl,
  facets,
  revision,
  enabled,
}: {
  baseUrl: string
  facets?: Facets
  revision: number
  enabled: boolean
}) {
  const {
    state: { query },
    dispatch,
    defaults,
  } = useDashboardState()
  const [search, setSearch] = useState('')
  const [debounced, setDebounced] = useState('')
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(search), 200)
    return () => clearTimeout(timer)
  }, [search])
  const sessionFacets = useFacets(baseUrl, query, revision, enabled && debounced !== '', debounced)
  const hasFilters =
    dimensions.some((d) => query[d.key].length > 0) || Boolean(query.from || query.to)
  return (
    <section className="filters" aria-label="Dashboard filters">
      <div className="filter-toolbar">
        <div className="filter-group">
          <span className="filter-caption">
            <Filter size="1em" />
            Filter
          </span>
          {dimensions.map((d) => (
            <MultiSelect
              key={d.key}
              label={d.label}
              selected={query[d.key]}
              values={
                (d.key === 'sessions' && debounced ? sessionFacets.data : facets)?.[d.key] ?? []
              }
              onChange={(values) => dispatch({ type: 'selection', value: { [d.key]: values } })}
              onSearch={d.key === 'sessions' ? setSearch : undefined}
              loading={d.key === 'sessions' && sessionFacets.isFetching}
            />
          ))}
        </div>
        <div className="filter-group">
          <DateFilter />
          <button
            className="icon-button"
            title="Restore CLI defaults"
            aria-label="Restore CLI defaults"
            onClick={() => dispatch({ type: 'selection', value: defaults })}
          >
            <RotateCcw size="1.1em" />
          </button>
        </div>
      </div>
      {hasFilters && (
        <div className="active-filters" aria-label="Active filters">
          {dimensions.flatMap((d) =>
            query[d.key].map((value) => (
              <button
                key={`${d.key}:${value}`}
                className="filter-chip"
                onClick={() =>
                  dispatch({
                    type: 'selection',
                    value: { [d.key]: query[d.key].filter((v) => v !== value) },
                  })
                }
                aria-label={`Remove ${d.label.toLowerCase()} ${value}`}
              >
                <span className="muted">{d.label}</span>
                <span className="chip-value" title={value}>
                  {value}
                </span>
                <X size="0.9em" />
              </button>
            )),
          )}
          {(query.from || query.to) && (
            <button
              className="filter-chip"
              onClick={() => dispatch({ type: 'selection', value: { from: '', to: '' } })}
            >
              Custom dates
              <X size="0.9em" />
            </button>
          )}
          <button
            className="text-button"
            onClick={() =>
              dispatch({
                type: 'selection',
                value: { providers: [], models: [], harnesses: [], sessions: [], from: '', to: '' },
              })
            }
          >
            Clear all
          </button>
        </div>
      )}
    </section>
  )
}

export function BucketControl() {
  const {
    state: { query },
    dispatch,
  } = useDashboardState()
  return (
    <label className="bucket-control">
      Bucket
      <select
        aria-label="Time bucket"
        value={query.bucket}
        onChange={(e) => {
          const parsed = bucketSchema.safeParse(e.target.value)
          if (parsed.success) dispatch({ type: 'selection', value: { bucket: parsed.data } })
        }}
      >
        {bucketSchema.options.map((bucket) => (
          <option key={bucket} value={bucket}>
            {bucket[0]?.toUpperCase()}
            {bucket.slice(1)}
          </option>
        ))}
      </select>
    </label>
  )
}

export function QuickPeriods() {
  const {
    state: { query },
    dispatch,
  } = useDashboardState()
  return (
    <div className="quick-periods" aria-label="Quick date ranges">
      {['today', 'week', 'month', 'year', 'all'].map((value) => {
        const period = periodSchema.parse(value)
        return (
          <button
            key={period}
            aria-pressed={query.period === period && !query.from && !query.to}
            onClick={() => dispatch({ type: 'selection', value: { period, from: '', to: '' } })}
          >
            {period === 'all' ? 'All time' : period[0]?.toUpperCase() + period.slice(1)}
          </button>
        )
      })}
    </div>
  )
}
