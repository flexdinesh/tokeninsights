import { useEffect, useId, useRef, useState } from 'react'
import { CalendarDays, Check, ChevronDown, Filter, Search, X } from 'lucide-react'
import type { Dimension, Facets, Selection } from '../contracts'
import { bucketSchema, periodSchema } from '../contracts'
import { useDashboardQuery } from '../state'
import { useFacets } from '../api'
import { Button } from './ui/button'
import { Checkbox } from './ui/checkbox'
import { Input } from './ui/input'
import { Popover, PopoverClose, PopoverContent, PopoverTrigger } from './ui/popover'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from './ui/select'

const dimensions: { key: Dimension; label: string }[] = [
  { key: 'harnesses', label: 'Harness' },
  { key: 'providers', label: 'Provider' },
  { key: 'models', label: 'Model' },
  { key: 'sessions', label: 'Session' },
]
const locationDimensions: {
  key: 'repositories' | 'directories'
  label: string
}[] = [
  { key: 'repositories', label: 'Repository' },
  { key: 'directories', label: 'Directory' },
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
  search: controlledSearch,
  loading = false,
  error,
  missingName,
}: {
  label: string
  values: (string | { key: string; name: string })[]
  selected: string[]
  onChange: (values: string[]) => void
  onSearch?: (search: string) => void
  search?: string
  loading?: boolean
  error?: { message: string; onRetry: () => void }
  missingName?: string
}) {
  const [localSearch, setLocalSearch] = useState('')
  const search = controlledSearch ?? localSearch
  const searchId = useId()
  const searchInput = useRef<HTMLInputElement>(null)
  const known = values.map((value) =>
    typeof value === 'string' ? { key: value, name: value } : value,
  )
  const fallbackName = (key: string) => (key === 'unknown' ? 'unknown' : (missingName ?? key))
  const options = [
    ...known,
    ...selected
      .filter((key) => !known.some((value) => value.key === key))
      .map((key) => ({ key, name: fallbackName(key) })),
  ]
    // oxlint-disable-next-line unicorn/no-array-sort -- This array is freshly created.
    .sort((a, b) => a.name.localeCompare(b.name))
    .filter((value) => value.name.toLowerCase().includes(search.toLowerCase()))
  const selectedNames = selected.map(
    (key) => known.find((value) => value.key === key)?.name ?? fallbackName(key),
  )
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className={`filter-button ${selected.length ? 'is-selected' : ''}`}
          aria-label={`${label}${selected.length ? ` ${selected.length}` : ''}`}
        >
          <span className="filter-label">{label}:</span>
          <span className="filter-selection" aria-hidden="true">
            {selected.length ? selectedNames.join(', ') : 'All'}
          </span>
          {selected.length > 0 && <span className="count-badge">{selected.length}</span>}
          <ChevronDown size="1em" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        aria-label={`${label} filter`}
        onOpenAutoFocus={(event) => {
          event.preventDefault()
          searchInput.current?.focus()
        }}
      >
        <div className="popover-heading">
          <strong>{label}</strong>
          <Button variant="ghost" size="sm" className="text-button" onClick={() => onChange([])}>
            Clear
          </Button>
        </div>
        <label className="search-field" htmlFor={searchId}>
          <Search size="1em" />
          <Input
            ref={searchInput}
            id={searchId}
            aria-label={`Search ${label.toLowerCase()}`}
            placeholder={`Find ${label.toLowerCase()}…`}
            value={search}
            onChange={(e) => {
              if (controlledSearch === undefined) setLocalSearch(e.target.value)
              onSearch?.(e.target.value)
            }}
          />
        </label>
        <div className="filter-options" aria-busy={loading}>
          {options.map((value, index) => (
            <label className="check-option" key={value.key} htmlFor={`${searchId}-${index}`}>
              <Checkbox
                id={`${searchId}-${index}`}
                checked={selected.includes(value.key)}
                onCheckedChange={() =>
                  onChange(
                    selected.includes(value.key)
                      ? selected.filter((v) => v !== value.key)
                      : [...selected, value.key],
                  )
                }
              />
              <span title={value.name}>{value.name}</span>
            </label>
          ))}
          {options.length === 0 && !error && (
            <p className="muted">{loading ? 'Loading…' : 'No matching values'}</p>
          )}
        </div>
        {error && (
          <div role="alert">
            <p className="error-text">{error.message}</p>
            <Button variant="outline" size="sm" onClick={error.onRetry} disabled={loading}>
              Retry {label.toLowerCase()} search
            </Button>
          </div>
        )}
        {onSearch && <p className="hint">First 100 matches. Search to narrow results.</p>}
        <PopoverClose asChild>
          <Button className="full-width">
            <Check size="1em" />
            Done
          </Button>
        </PopoverClose>
      </PopoverContent>
    </Popover>
  )
}

function SessionFilter({
  values,
  revision,
  enabled,
  identity,
}: {
  values: string[]
  revision: number
  enabled: boolean
  identity: string
}) {
  const { query, updateSelection } = useDashboardQuery()
  const [search, setSearch] = useState('')
  const [debounced, setDebounced] = useState('')
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(search), 200)
    return () => clearTimeout(timer)
  }, [search])
  const sessionFacets = useFacets(query, revision, enabled && debounced !== '', identity, debounced)
  const current = search === debounced
  const searching = search !== ''
  return (
    <MultiSelect
      label="Session"
      selected={query.sessions}
      values={searching ? (current ? (sessionFacets.data?.sessions ?? []) : []) : values}
      onChange={(sessions) => updateSelection({ sessions })}
      search={search}
      onSearch={setSearch}
      loading={searching && (!current || sessionFacets.isFetching)}
      error={
        searching && current && sessionFacets.error
          ? {
              message: 'Session values couldn’t load.',
              onRetry: () => void sessionFacets.refetch(),
            }
          : undefined
      }
    />
  )
}

function DateFilter() {
  const { query, updateSelection } = useDashboardQuery()
  const errorId = useId()
  const [open, setOpen] = useState(false)
  const [from, setFrom] = useState(query.from)
  const [to, setTo] = useState(query.to)
  const invalid = Boolean(from && to && from > to)
  const custom = query.from || query.to
  return (
    <Popover
      open={open}
      onOpenChange={(value) => {
        setOpen(value)
        if (value) {
          setFrom(query.from)
          setTo(query.to)
        }
      }}
    >
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="filter-button date-button">
          <CalendarDays size="1em" />
          {custom
            ? `${query.from || 'Beginning'} → ${query.to || 'Now'}`
            : periods.find((p) => p.value === query.period)?.label}
          <ChevronDown size="1em" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="date-popover" align="end" aria-label="Date range">
        <strong>Date range</strong>
        <div className="date-presets">
          {periods.map((p) => (
            <Button
              key={p.value}
              variant={!custom && query.period === p.value ? 'secondary' : 'outline'}
              aria-pressed={!custom && query.period === p.value}
              onClick={() => {
                updateSelection({ period: p.value, from: '', to: '' })
                setOpen(false)
              }}
            >
              {p.label}
            </Button>
          ))}
        </div>
        <div className="date-inputs">
          <label htmlFor={`${errorId}-from`}>
            From
            <Input
              id={`${errorId}-from`}
              aria-label="From date"
              aria-invalid={invalid || undefined}
              aria-describedby={invalid ? errorId : undefined}
              type="date"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
            />
          </label>
          <label htmlFor={`${errorId}-to`}>
            To
            <Input
              id={`${errorId}-to`}
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
        <Button
          className="full-width"
          disabled={invalid || (!from && !to)}
          onClick={() => {
            updateSelection({ from, to })
            setOpen(false)
          }}
        >
          Apply range
        </Button>
      </PopoverContent>
    </Popover>
  )
}

export function FilterToolbar({
  facets,
  revision,
  enabled,
  identity,
}: {
  facets?: Facets
  revision: number
  enabled: boolean
  identity: string
}) {
  const { query, updateSelection, updateLocations, clearFilters } = useDashboardQuery()
  const hasFilters =
    dimensions.some((d) => query[d.key].length > 0) ||
    (query.tab === 'repo' && locationDimensions.some((d) => query[d.key].length > 0)) ||
    Boolean(query.from || query.to)
  return (
    <section className="filters" aria-label="Dashboard filters">
      <div className="filter-content">
        <div className="filter-toolbar">
          <span className="filter-caption">
            <Filter size="1em" /> Filters
          </span>
          <div className="filter-date">
            <DateFilter />
          </div>
          <div className="filter-group">
            {dimensions.map((d) =>
              d.key === 'sessions' ? (
                <SessionFilter
                  key={d.key}
                  values={facets?.sessions ?? []}
                  revision={revision}
                  enabled={enabled}
                  identity={identity}
                />
              ) : (
                <MultiSelect
                  key={d.key}
                  label={d.label}
                  selected={query[d.key]}
                  values={facets?.[d.key] ?? []}
                  onChange={(values) => updateSelection({ [d.key]: values })}
                />
              ),
            )}
            {query.tab === 'repo' &&
              locationDimensions.map((d) => (
                <MultiSelect
                  key={d.key}
                  label={d.label}
                  selected={query[d.key]}
                  values={facets?.[d.key] ?? []}
                  missingName={`${d.label} (unavailable)`}
                  onChange={(values) => updateLocations({ [d.key]: values })}
                />
              ))}
          </div>
        </div>
        {hasFilters && (
          <div className="active-filters" aria-label="Active filters">
            {dimensions.flatMap((d) =>
              query[d.key].map((value) => (
                <Button
                  key={`${d.key}:${value}`}
                  variant="secondary"
                  size="sm"
                  className="filter-chip"
                  onClick={() =>
                    updateSelection({ [d.key]: query[d.key].filter((v) => v !== value) })
                  }
                  aria-label={`Remove ${d.label.toLowerCase()} ${value}`}
                >
                  <span className="muted">{d.label}</span>
                  <span className="chip-value" title={value}>
                    {value}
                  </span>
                  <X size="0.9em" />
                </Button>
              )),
            )}
            {query.tab === 'repo' &&
              locationDimensions.flatMap((d) =>
                query[d.key].map((value) => {
                  const name =
                    facets?.[d.key].find((option) => option.key === value)?.name ??
                    (value === 'unknown' ? 'unknown' : `${d.label} (unavailable)`)
                  return (
                    <Button
                      key={`${d.key}:${value}`}
                      variant="secondary"
                      size="sm"
                      className="filter-chip"
                      onClick={() =>
                        updateLocations({ [d.key]: query[d.key].filter((key) => key !== value) })
                      }
                      aria-label={`Remove ${d.label.toLowerCase()} ${name}`}
                    >
                      <span className="muted">{d.label}</span>
                      <span className="chip-value" title={name}>
                        {name}
                      </span>
                      <X size="0.9em" />
                    </Button>
                  )
                }),
              )}
            {(query.from || query.to) && (
              <Button
                variant="secondary"
                size="sm"
                className="filter-chip"
                onClick={() => updateSelection({ from: '', to: '' })}
              >
                Custom dates
                <X size="0.9em" />
              </Button>
            )}
            <Button variant="ghost" size="sm" className="text-button" onClick={clearFilters}>
              Clear All
            </Button>
          </div>
        )}
        <p className="sr-only">
          Date and usage filters apply to every view. Location filters apply only to Repo. Sync
          always refreshes all supported harnesses.
        </p>
      </div>
    </section>
  )
}

export function BucketControl() {
  const { query, updateSelection } = useDashboardQuery()
  return (
    <div className="bucket-control">
      Bucket
      <Select
        value={query.bucket}
        onValueChange={(value) => {
          const parsed = bucketSchema.safeParse(value)
          if (parsed.success) updateSelection({ bucket: parsed.data })
        }}
      >
        <SelectTrigger size="sm" aria-label="Time bucket">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {bucketSchema.options.map((bucket) => (
            <SelectItem key={bucket} value={bucket}>
              {bucket[0]?.toUpperCase()}
              {bucket.slice(1)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

export function QuickPeriods() {
  const { query, updateSelection } = useDashboardQuery()
  return (
    <div className="quick-periods" aria-label="Quick date ranges">
      {['today', 'week', 'month', 'year', 'all'].map((value) => {
        const period = periodSchema.parse(value)
        return (
          <Button
            key={period}
            variant="ghost"
            size="sm"
            aria-pressed={query.period === period && !query.from && !query.to}
            onClick={() => updateSelection({ period, from: '', to: '' })}
          >
            {period === 'all' ? 'All time' : period[0]?.toUpperCase() + period.slice(1)}
          </Button>
        )
      })}
    </div>
  )
}
