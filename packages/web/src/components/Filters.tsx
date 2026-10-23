import { useEffect, useId, useRef, useState } from 'react'
import { CalendarDays, Check, ChevronDown, Filter } from 'lucide-react'
import type { Dimension, Facets, Selection } from '../contracts'
import { bucketSchema } from '../contracts'
import { useDashboardQuery } from '../state'
import { useFacets } from '../api'
import {
  Badge,
  Button,
  CheckboxOption,
  Chip,
  ControlLabel,
  Field,
  Input,
  Popover,
  PopoverClose,
  PopoverContent,
  PopoverHeader,
  PopoverTrigger,
  SearchField,
  SegmentedControl,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Text,
} from '@/ui'

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
  { value: 'today', label: 'Daily' },
  { value: 'week', label: 'Weekly' },
  { value: 'month', label: 'Monthly' },
  { value: 'year', label: 'Yearly' },
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
          size="compact"
          selected={selected.length > 0}
          className="max-w-full gap-1 max-compact:whitespace-normal"
          aria-label={`${label}${selected.length ? ` ${selected.length}` : ''}`}
        >
          <span>{label}:</span>
          <span
            className="max-w-(--filter-selection-width) truncate text-ink-subtle"
            aria-hidden="true"
          >
            {selected.length ? selectedNames.join(', ') : 'All'}
          </span>
          {selected.length > 0 && <Badge>{selected.length}</Badge>}
          <ChevronDown aria-hidden="true" />
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
        <PopoverHeader
          title={label}
          action={
            <Button variant="link" size="compact" onClick={() => onChange([])}>
              Clear
            </Button>
          }
        />
        <SearchField
          ref={searchInput}
          id={searchId}
          className="my-2"
          aria-label={`Search ${label.toLowerCase()}`}
          placeholder={`Find ${label.toLowerCase()}…`}
          value={search}
          onChange={(e) => {
            if (controlledSearch === undefined) setLocalSearch(e.target.value)
            onSearch?.(e.target.value)
          }}
        />
        <div className="mb-2 max-h-(--filter-options-max-height) overflow-auto" aria-busy={loading}>
          {options.map((value, index) => (
            <CheckboxOption
              key={value.key}
              id={`${searchId}-${index}`}
              label={value.name}
              title={value.name}
              checked={selected.includes(value.key)}
              onCheckedChange={() =>
                onChange(
                  selected.includes(value.key)
                    ? selected.filter((v) => v !== value.key)
                    : [...selected, value.key],
                )
              }
            />
          ))}
          {options.length === 0 && !error && (
            <Text variant="muted">{loading ? 'Loading…' : 'No matching values'}</Text>
          )}
        </div>
        {error && (
          <div role="alert">
            <Text variant="danger" className="mb-2">
              {error.message}
            </Text>
            <Button variant="outline" size="compact" onClick={error.onRetry} disabled={loading}>
              Retry {label.toLowerCase()} search
            </Button>
          </div>
        )}
        {onSearch && (
          <Text variant="muted" className="my-2">
            First 100 matches. Search to narrow results.
          </Text>
        )}
        <PopoverClose asChild>
          <Button className="w-full">
            <Check aria-hidden="true" />
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
  const custom = Boolean((query.from || query.to) && query.dateMode !== 'calendar')
  const [editingCustom, setEditingCustom] = useState(custom)
  const dateLabel = custom
    ? `${query.from || 'Beginning'} → ${query.to || 'Now'}`
    : (periods.find((p) => p.value === query.period)?.label ?? 'Daily')
  return (
    <Popover
      open={open}
      onOpenChange={(value) => {
        setOpen(value)
        if (value) {
          setEditingCustom(custom)
          setFrom(custom ? query.from : '')
          setTo(custom ? query.to : '')
        }
      }}
    >
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="compact"
          className="max-w-full whitespace-normal wrap-anywhere"
          aria-label={`Date range: ${dateLabel}`}
        >
          <CalendarDays aria-hidden="true" />
          {dateLabel}
          <ChevronDown aria-hidden="true" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" aria-label="Date range">
        <PopoverHeader title="Date range" />
        <SegmentedControl
          label="Date presets"
          variant="outline"
          size="default"
          className="mt-2 mb-3 grid grid-cols-2 gap-2"
          options={[...periods, { value: 'custom', label: 'Custom' }]}
          value={editingCustom ? 'custom' : query.period === 'yesterday' ? 'today' : query.period}
          onValueChange={(value) => {
            if (value === 'custom') {
              setEditingCustom(true)
              return
            }
            updateSelection({ period: value, from: '', to: '', dateMode: undefined })
            setOpen(false)
          }}
        />
        {editingCustom && (
          <>
            <div className="grid grid-cols-2 gap-3 max-compact:grid-cols-1">
              <Field label="From">
                {(control) => (
                  <Input
                    {...control}
                    aria-label="From date"
                    aria-invalid={invalid || undefined}
                    aria-describedby={invalid ? errorId : undefined}
                    type="date"
                    className="w-full"
                    value={from}
                    onChange={(e) => setFrom(e.target.value)}
                  />
                )}
              </Field>
              <Field label="To">
                {(control) => (
                  <Input
                    {...control}
                    aria-label="To date"
                    aria-invalid={invalid || undefined}
                    aria-describedby={invalid ? errorId : undefined}
                    type="date"
                    className="w-full"
                    value={to}
                    onChange={(e) => setTo(e.target.value)}
                  />
                )}
              </Field>
            </div>
            <Text variant="muted" className="my-2">
              Inclusive dates in the server’s local timezone. Either bound can be empty.
            </Text>
            {invalid && (
              <Text id={errorId} role="alert" variant="danger" className="mb-2">
                From must not be after to.
              </Text>
            )}
            <Button
              className="w-full"
              disabled={invalid || (!from && !to)}
              onClick={() => {
                updateSelection({ from, to, dateMode: undefined })
                setOpen(false)
              }}
            >
              Apply range
            </Button>
          </>
        )}
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
    Boolean((query.from || query.to) && query.dateMode !== 'calendar')
  return (
    <section className="min-w-0 flex-auto" aria-label="Dashboard filters">
      <div className="flex flex-wrap items-center gap-2">
        <span className="mr-2 flex items-center gap-2 text-label text-ink-subtle max-compact:hidden [&_svg]:size-(--icon-size)">
          <Filter aria-hidden="true" /> Filters
        </span>
        <div className="min-w-0">
          <DateFilter />
        </div>
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
      {hasFilters && (
        <div className="mt-2 flex flex-wrap gap-2" role="group" aria-label="Active filters">
          {dimensions.flatMap((d) =>
            query[d.key].map((value) => (
              <Chip
                key={`${d.key}:${value}`}
                dimension={d.label}
                value={value}
                title={value}
                removeLabel={`Remove ${d.label.toLowerCase()} ${value}`}
                onRemove={() =>
                  updateSelection({ [d.key]: query[d.key].filter((v) => v !== value) })
                }
              />
            )),
          )}
          {query.tab === 'repo' &&
            locationDimensions.flatMap((d) =>
              query[d.key].map((value) => {
                const name =
                  facets?.[d.key].find((option) => option.key === value)?.name ??
                  (value === 'unknown' ? 'unknown' : `${d.label} (unavailable)`)
                return (
                  <Chip
                    key={`${d.key}:${value}`}
                    dimension={d.label}
                    value={name}
                    title={name}
                    removeLabel={`Remove ${d.label.toLowerCase()} ${name}`}
                    onRemove={() =>
                      updateLocations({ [d.key]: query[d.key].filter((key) => key !== value) })
                    }
                  />
                )
              }),
            )}
          {(query.from || query.to) && query.dateMode !== 'calendar' && (
            <Chip
              value="Custom dates"
              onRemove={() => updateSelection({ from: '', to: '', dateMode: undefined })}
            />
          )}
          <Button variant="link" size="compact" onClick={clearFilters}>
            Clear All
          </Button>
        </div>
      )}
      <p className="sr-only">
        Date and usage filters apply to every view. Location filters apply only to Repo. Sync always
        refreshes all supported harnesses.
      </p>
    </section>
  )
}

export function BucketControl() {
  const { query, updateSelection } = useDashboardQuery()
  return (
    <ControlLabel label="Bucket">
      <Select
        value={query.bucket}
        onValueChange={(value) => {
          const parsed = bucketSchema.safeParse(value)
          if (parsed.success) updateSelection({ bucket: parsed.data })
        }}
      >
        <SelectTrigger size="compact" aria-label="Time bucket">
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
    </ControlLabel>
  )
}
