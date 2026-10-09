import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { z } from 'zod'
import { bucketSchema, locationGroupSchema, periodSchema, sortSchema } from './contracts'
import type { LocationGroup, Selection, Sort, Tab } from './contracts'

export interface QueryState extends Selection {
  quality: 'confirmed' | 'estimated'
  tab: Tab
  locationGroup: LocationGroup
  repositories: string[]
  directories: string[]
  sort: Sort
  direction: 'asc' | 'desc'
  page: number
  pageSize: number
}

export interface DashboardSearch {
  quality?: 'confirmed' | 'estimated'
  v?: 1
  period?: Selection['period']
  bucket?: Selection['bucket']
  from?: string
  to?: string
  providers: string[]
  models: string[]
  harnesses: Selection['harnesses']
  sessions: string[]
  locationGroup?: LocationGroup
  repositories: string[]
  directories: string[]
  sort?: Sort
  direction?: 'asc' | 'desc'
  page?: number
  pageSize?: number
}

const themeSchema = z.enum(['system', 'light', 'dark'])
export type Theme = z.infer<typeof themeSchema>
export type QueryAction =
  | { type: 'quality'; value: 'confirmed' | 'estimated' }
  | { type: 'selection'; value: Partial<Selection> }
  | { type: 'reset'; value: Selection }
  | { type: 'clearFilters' }
  | { type: 'tab'; value: Tab }
  | { type: 'locationGroup'; value: LocationGroup }
  | {
      type: 'locations'
      value: Partial<Pick<QueryState, 'repositories' | 'directories'>>
    }
  | { type: 'sort'; value: Sort }
  | { type: 'page'; value: number }
  | { type: 'pageSize'; value: number }

export function defaultSort(tab: Tab): Sort {
  return tab === 'context'
    ? 'averageContext'
    : tab === 'tokens' || tab === 'sessions'
      ? 'date'
      : 'total'
}

export function initialQuery(defaults: Selection, tab: Tab = 'tokens'): QueryState {
  return {
    ...defaults,
    quality: 'confirmed',
    tab,
    locationGroup: 'repository',
    repositories: [],
    directories: [],
    sort: defaultSort(tab),
    direction: 'desc',
    page: 1,
    pageSize: 50,
  }
}

export function reduceQuery(query: QueryState, action: QueryAction): QueryState {
  switch (action.type) {
    case 'quality':
      return { ...query, quality: action.value, page: 1 }
    case 'selection':
      return { ...query, ...action.value, page: 1 }
    case 'reset':
      return {
        ...query,
        ...action.value,
        repositories: [],
        directories: [],
        page: 1,
      }
    case 'clearFilters':
      return {
        ...query,
        providers: [],
        models: [],
        harnesses: [],
        sessions: [],
        repositories: [],
        directories: [],
        from: '',
        to: '',
        page: 1,
      }
    case 'tab':
      return {
        ...query,
        tab: action.value,
        repositories: action.value === 'repo' ? query.repositories : [],
        directories: action.value === 'repo' ? query.directories : [],
        sort: defaultSort(action.value),
        direction: 'desc',
        page: 1,
      }
    case 'locationGroup':
      return { ...query, locationGroup: action.value, page: 1 }
    case 'locations':
      return { ...query, ...action.value, page: 1 }
    case 'sort':
      return {
        ...query,
        sort: action.value,
        direction:
          query.sort === action.value
            ? query.direction === 'asc'
              ? 'desc'
              : 'asc'
            : ['name', 'harness', 'provider', 'model'].includes(action.value)
              ? 'asc'
              : 'desc',
        page: 1,
      }
    case 'page':
      return { ...query, page: action.value }
    case 'pageSize':
      return { ...query, pageSize: action.value, page: 1 }
    default:
      return query
  }
}

function strings(value: unknown): string[] {
  if (typeof value === 'string') return [value]
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string')
}

function positive(value: unknown, max: number): number | undefined {
  const parsed = typeof value === 'number' ? value : Number(value)
  return Number.isSafeInteger(parsed) && parsed > 0 && parsed <= max ? parsed : undefined
}

function date(value: unknown): string | undefined {
  return typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value) ? value : undefined
}

export function parseDashboardSearch(input: Record<string, unknown>): DashboardSearch {
  const period = periodSchema.safeParse(input.period)
  const bucket = bucketSchema.safeParse(input.bucket)
  const sort = sortSchema.safeParse(input.sort)
  const locationGroup = locationGroupSchema.safeParse(input.locationGroup)
  const harnesses = strings(input.harnesses ?? input.harness)
  return {
    quality: input.quality === 'estimated' ? 'estimated' : undefined,
    v: input.v === 1 || input.v === '1' ? 1 : undefined,
    period: period.success ? period.data : undefined,
    bucket: bucket.success ? bucket.data : undefined,
    from: date(input.from),
    to: date(input.to),
    providers: strings(input.providers ?? input.provider),
    models: strings(input.models ?? input.model),
    harnesses: harnesses.filter((value): value is Selection['harnesses'][number] =>
      ['opencode', 'pi', 'codex', 'claude-code'].includes(value),
    ),
    sessions: strings(input.sessions ?? input.session),
    locationGroup: locationGroup.success ? locationGroup.data : undefined,
    repositories: strings(input.repositories ?? input.repository),
    directories: strings(input.directories ?? input.directory),
    sort: sort.success ? sort.data : undefined,
    direction:
      input.direction === 'asc' || input.direction === 'desc' ? input.direction : undefined,
    page: positive(input.page, Number.MAX_SAFE_INTEGER),
    pageSize: positive(input.pageSize, 200),
  }
}

export function parseDashboardSearchParams(value: string): DashboardSearch {
  const params = new URLSearchParams(value.startsWith('?') ? value.slice(1) : value)
  return parseDashboardSearch({
    quality: params.get('quality'),
    v: params.get('v'),
    period: params.get('period'),
    bucket: params.get('bucket'),
    from: params.get('from'),
    to: params.get('to'),
    providers: params.getAll('provider'),
    models: params.getAll('model'),
    harnesses: params.getAll('harness'),
    sessions: params.getAll('session'),
    locationGroup: params.get('locationGroup'),
    repositories: params.getAll('repository'),
    directories: params.getAll('directory'),
    sort: params.get('sort'),
    direction: params.get('direction'),
    page: params.get('page'),
    pageSize: params.get('pageSize'),
    tab: params.get('tab'),
  })
}

export function stringifyDashboardSearch(
  search: Record<string, unknown> | DashboardSearch,
): string {
  const params = new URLSearchParams()
  const scalar = (key: string, value: unknown) => {
    if (typeof value === 'string' || typeof value === 'number') params.set(key, String(value))
  }
  scalar('v', search.v)
  scalar('quality', search.quality)
  scalar('period', search.period)
  scalar('bucket', search.bucket)
  scalar('from', search.from)
  scalar('to', search.to)
  scalar('locationGroup', search.locationGroup)
  const repeated: [string, unknown][] = [
    ['provider', search.providers],
    ['model', search.models],
    ['harness', search.harnesses],
    ['session', search.sessions],
    ['repository', search.repositories],
    ['directory', search.directories],
  ]
  for (const [key, value] of repeated) {
    for (const item of strings(value)) params.append(key, item)
  }
  scalar('sort', search.sort)
  scalar('direction', search.direction)
  scalar('page', search.page)
  scalar('pageSize', search.pageSize)
  const value = params.toString()
  return value === '' ? '' : `?${value}`
}

export function searchFromQuery(query: QueryState): DashboardSearch {
  return {
    quality: query.quality === 'estimated' ? 'estimated' : undefined,
    v: 1,
    period: query.period,
    bucket: query.bucket,
    from: query.from || undefined,
    to: query.to || undefined,
    providers: query.providers,
    models: query.models,
    harnesses: query.harnesses,
    sessions: query.sessions,
    locationGroup: query.tab === 'repo' ? query.locationGroup : undefined,
    repositories: query.tab === 'repo' ? query.repositories : [],
    directories: query.tab === 'repo' ? query.directories : [],
    sort: query.sort,
    direction: query.direction,
    page: query.page,
    pageSize: query.pageSize,
  }
}

function configured(search: DashboardSearch): boolean {
  return Boolean(
    search.quality ||
    search.v ||
    search.period ||
    search.bucket ||
    search.from ||
    search.to ||
    search.providers.length ||
    search.models.length ||
    search.harnesses.length ||
    search.sessions.length ||
    search.locationGroup ||
    search.repositories.length ||
    search.directories.length ||
    search.sort ||
    search.direction ||
    search.page ||
    search.pageSize,
  )
}

export function queryFromSearch(
  search: DashboardSearch,
  tab: Tab,
  defaults: Selection,
): QueryState {
  if (!configured(search)) return initialQuery(defaults, tab)
  let sort = search.sort ?? defaultSort(tab)
  const contextSorts: Sort[] = [
    'averageContext',
    'medianContext',
    'maxContext',
    'sessions',
    'harness',
    'provider',
    'model',
  ]
  if (
    tab === 'context'
      ? !contextSorts.includes(sort)
      : contextSorts.some((value) => value !== 'sessions' && value === sort)
  ) {
    sort = defaultSort(tab)
  }
  return {
    period: search.period ?? defaults.period,
    quality: search.quality ?? 'confirmed',
    bucket: search.bucket ?? defaults.bucket,
    from: search.from ?? '',
    to: search.to ?? '',
    providers: search.providers,
    models: search.models,
    harnesses: search.harnesses,
    sessions: search.sessions,
    locationGroup: tab === 'repo' ? (search.locationGroup ?? 'repository') : 'repository',
    repositories: tab === 'repo' ? search.repositories : [],
    directories: tab === 'repo' ? search.directories : [],
    tab,
    sort,
    direction: search.direction ?? 'desc',
    page: search.page ?? 1,
    pageSize: search.pageSize ?? 50,
  }
}

export function apiQueryParams(query: QueryState): URLSearchParams {
  const params = new URLSearchParams({
    period: query.period,
    bucket: query.bucket,
    tab: query.tab,
    sort: query.sort,
    direction: query.direction,
    page: String(query.page),
    pageSize: String(query.pageSize),
  })
  if (query.quality === 'estimated') params.set('quality', 'estimated')
  if (query.from) params.set('from', query.from)
  if (query.to) params.set('to', query.to)
  for (const value of query.providers) params.append('provider', value)
  for (const value of query.models) params.append('model', value)
  for (const value of query.harnesses) params.append('harness', value)
  for (const value of query.sessions) params.append('session', value)
  if (query.tab === 'repo') {
    params.set('locationGroup', query.locationGroup)
    for (const value of query.repositories) params.append('repository', value)
    for (const value of query.directories) params.append('directory', value)
  }
  return params
}

const QueryContext = createContext<{
  query: QueryState
  setQuality: (value: QueryState['quality']) => void
  defaults: Selection
  updateSelection: (value: Partial<Selection>) => void
  resetSelection: () => void
  clearFilters: () => void
  setLocationGroup: (value: LocationGroup) => void
  updateLocations: (value: Partial<Pick<QueryState, 'repositories' | 'directories'>>) => void
  sortBy: (value: Sort) => void
  setPage: (value: number) => void
  setPageSize: (value: number) => void
} | null>(null)

const PreferencesContext = createContext<{
  theme: Theme
  hiddenColumns: string[]
  chartMetric: Sort
  setTheme: (value: Theme) => void
  toggleColumn: (value: string) => void
  setChartMetric: (value: Sort) => void
} | null>(null)

function savedTheme(): Theme {
  try {
    return themeSchema.catch('system').parse(localStorage.getItem('tokeninsights.theme'))
  } catch {
    return 'system'
  }
}

export function DashboardProvider({
  defaults,
  children,
}: {
  defaults: Selection
  children: ReactNode
}) {
  return (
    <DashboardQueryProvider defaults={defaults}>
      <DashboardPreferencesProvider>{children}</DashboardPreferencesProvider>
    </DashboardQueryProvider>
  )
}

function DashboardQueryProvider({
  defaults,
  children,
}: {
  defaults: Selection
  children: ReactNode
}) {
  const { tab } = useParams({ from: '/$tab' })
  const search = useSearch({ from: '/$tab' })
  const navigate = useNavigate({ from: '/$tab' })
  const query = useMemo(() => queryFromSearch(search, tab, defaults), [defaults, search, tab])
  const canonicalSearch = useMemo(() => searchFromQuery(query), [query])
  const currentSearchKey = stringifyDashboardSearch(search)
  const canonicalSearchKey = stringifyDashboardSearch(canonicalSearch)

  useEffect(() => {
    if (currentSearchKey !== canonicalSearchKey) {
      void navigate({ to: '/$tab', params: { tab }, search: canonicalSearch, replace: true })
    }
  }, [canonicalSearch, canonicalSearchKey, currentSearchKey, navigate, tab])

  const navigateQuery = useCallback(
    (action: QueryAction) => {
      const next = reduceQuery(query, action)
      void navigate({
        to: '/$tab',
        params: { tab: next.tab },
        search: searchFromQuery(next),
        resetScroll: false,
      })
    },
    [navigate, query],
  )

  const contextValue = useMemo(
    () => ({
      query,
      setQuality: (value: QueryState['quality']) => navigateQuery({ type: 'quality', value }),
      defaults,
      updateSelection: (value: Partial<Selection>) => navigateQuery({ type: 'selection', value }),
      resetSelection: () => navigateQuery({ type: 'reset', value: defaults }),
      clearFilters: () => navigateQuery({ type: 'clearFilters' }),
      setLocationGroup: (value: LocationGroup) => navigateQuery({ type: 'locationGroup', value }),
      updateLocations: (value: Partial<Pick<QueryState, 'repositories' | 'directories'>>) =>
        navigateQuery({ type: 'locations', value }),
      sortBy: (value: Sort) => navigateQuery({ type: 'sort', value }),
      setPage: (value: number) => navigateQuery({ type: 'page', value }),
      setPageSize: (value: number) => navigateQuery({ type: 'pageSize', value }),
    }),
    [defaults, navigateQuery, query],
  )
  return <QueryContext.Provider value={contextValue}>{children}</QueryContext.Provider>
}

function DashboardPreferencesProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState(savedTheme)
  const [hiddenColumns, setHiddenColumns] = useState<string[]>([])
  const [chartMetric, setChartMetric] = useState<Sort>('total')
  useEffect(() => {
    document.documentElement.dataset.theme = theme
    try {
      localStorage.setItem('tokeninsights.theme', theme)
    } catch {
      /* Preferences still work without storage. */
    }
  }, [theme])
  const toggleColumn = useCallback((column: string) => {
    setHiddenColumns((values) =>
      values.includes(column) ? values.filter((value) => value !== column) : [...values, column],
    )
  }, [])
  const value = useMemo(
    () => ({ theme, hiddenColumns, chartMetric, setTheme, toggleColumn, setChartMetric }),
    [chartMetric, hiddenColumns, theme, toggleColumn],
  )
  return <PreferencesContext.Provider value={value}>{children}</PreferencesContext.Provider>
}

export function useDashboardQuery() {
  const value = useContext(QueryContext)
  if (!value) throw new Error('DashboardQueryProvider missing')
  return value
}

export function useDashboardPreferences() {
  const value = useContext(PreferencesContext)
  if (!value) throw new Error('DashboardPreferencesProvider missing')
  return value
}
