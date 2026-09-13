import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { z } from 'zod'
import { bucketSchema, periodSchema, sortSchema, tabSchema } from './contracts'
import type { Selection, Sort, Tab } from './contracts'

export interface QueryState extends Selection {
  tab: Tab
  sort: Sort
  direction: 'asc' | 'desc'
  page: number
  pageSize: number
}

export interface DashboardSearch {
  v?: 1
  period?: Selection['period']
  bucket?: Selection['bucket']
  from?: string
  to?: string
  providers: string[]
  models: string[]
  harnesses: Selection['harnesses']
  sessions: string[]
  sort?: Sort
  direction?: 'asc' | 'desc'
  page?: number
  pageSize?: number
  legacyTab?: Tab
}

const themeSchema = z.enum(['system', 'light', 'dark'])
export type Theme = z.infer<typeof themeSchema>
interface State {
  query: QueryState
  theme: Theme
  hidden: string[]
  chartMetric: Sort
}
export type Action =
  | { type: 'selection'; value: Partial<Selection> }
  | { type: 'tab'; value: Tab }
  | { type: 'sort'; value: Sort }
  | { type: 'page'; value: number }
  | { type: 'pageSize'; value: number }
  | { type: 'theme'; value: Theme }
  | { type: 'column'; value: string }
  | { type: 'chartMetric'; value: Sort }

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
    tab,
    sort: defaultSort(tab),
    direction: 'desc',
    page: 1,
    pageSize: 50,
  }
}

export function reduceQuery(query: QueryState, action: Action): QueryState {
  switch (action.type) {
    case 'selection':
      return { ...query, ...action.value, page: 1 }
    case 'tab':
      return {
        ...query,
        tab: action.value,
        sort: defaultSort(action.value),
        direction: 'desc',
        page: 1,
      }
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
  const tab = tabSchema.safeParse(input.legacyTab ?? input.tab)
  const harnesses = strings(input.harnesses ?? input.harness)
  return {
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
    sort: sort.success ? sort.data : undefined,
    direction:
      input.direction === 'asc' || input.direction === 'desc' ? input.direction : undefined,
    page: positive(input.page, Number.MAX_SAFE_INTEGER),
    pageSize: positive(input.pageSize, 200),
    legacyTab: tab.success ? tab.data : undefined,
  }
}

export function parseDashboardSearchParams(value: string): DashboardSearch {
  const params = new URLSearchParams(value.startsWith('?') ? value.slice(1) : value)
  return parseDashboardSearch({
    v: params.get('v'),
    period: params.get('period'),
    bucket: params.get('bucket'),
    from: params.get('from'),
    to: params.get('to'),
    providers: params.getAll('provider'),
    models: params.getAll('model'),
    harnesses: params.getAll('harness'),
    sessions: params.getAll('session'),
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
  scalar('period', search.period)
  scalar('bucket', search.bucket)
  scalar('from', search.from)
  scalar('to', search.to)
  const repeated: [string, unknown][] = [
    ['provider', search.providers],
    ['model', search.models],
    ['harness', search.harnesses],
    ['session', search.sessions],
  ]
  for (const [key, value] of repeated) {
    for (const item of strings(value)) params.append(key, item)
  }
  scalar('sort', search.sort)
  scalar('direction', search.direction)
  scalar('page', search.page)
  scalar('pageSize', search.pageSize)
  scalar('tab', search.legacyTab)
  const value = params.toString()
  return value === '' ? '' : `?${value}`
}

export function searchFromQuery(query: QueryState): DashboardSearch {
  return {
    v: 1,
    period: query.period,
    bucket: query.bucket,
    from: query.from || undefined,
    to: query.to || undefined,
    providers: query.providers,
    models: query.models,
    harnesses: query.harnesses,
    sessions: query.sessions,
    sort: query.sort,
    direction: query.direction,
    page: query.page,
    pageSize: query.pageSize,
  }
}

function configured(search: DashboardSearch): boolean {
  return Boolean(
    search.v ||
    search.period ||
    search.bucket ||
    search.from ||
    search.to ||
    search.providers.length ||
    search.models.length ||
    search.harnesses.length ||
    search.sessions.length ||
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
    bucket: search.bucket ?? defaults.bucket,
    from: search.from ?? '',
    to: search.to ?? '',
    providers: search.providers,
    models: search.models,
    harnesses: search.harnesses,
    sessions: search.sessions,
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
  if (query.from) params.set('from', query.from)
  if (query.to) params.set('to', query.to)
  for (const value of query.providers) params.append('provider', value)
  for (const value of query.models) params.append('model', value)
  for (const value of query.harnesses) params.append('harness', value)
  for (const value of query.sessions) params.append('session', value)
  return params
}

const StateContext = createContext<{
  state: State
  dispatch: (action: Action) => void
  defaults: Selection
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
  const { tab } = useParams({ from: '/$tab' })
  const search = useSearch({ from: '/$tab' })
  const navigate = useNavigate({ from: '/$tab' })
  const query = useMemo(() => queryFromSearch(search, tab, defaults), [defaults, search, tab])
  const canonicalSearch = useMemo(() => searchFromQuery(query), [query])
  const currentSearchKey = stringifyDashboardSearch(search)
  const canonicalSearchKey = stringifyDashboardSearch(canonicalSearch)
  const [theme, setTheme] = useState(savedTheme)
  const [hidden, setHidden] = useState<string[]>([])
  const [chartMetric, setChartMetric] = useState<Sort>('total')

  useEffect(() => {
    if (currentSearchKey !== canonicalSearchKey) {
      void navigate({ to: '/$tab', params: { tab }, search: canonicalSearch, replace: true })
    }
  }, [canonicalSearch, canonicalSearchKey, currentSearchKey, navigate, tab])

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    try {
      localStorage.setItem('tokeninsights.theme', theme)
    } catch {
      /* Preferences still work without storage. */
    }
  }, [theme])

  const dispatch = useCallback(
    (action: Action) => {
      if (action.type === 'theme') {
        setTheme(action.value)
        return
      }
      if (action.type === 'column') {
        setHidden((values) =>
          values.includes(action.value)
            ? values.filter((value) => value !== action.value)
            : [...values, action.value],
        )
        return
      }
      if (action.type === 'chartMetric') {
        setChartMetric(action.value)
        return
      }
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

  const state = useMemo(
    () => ({ query, theme, hidden, chartMetric }),
    [chartMetric, hidden, query, theme],
  )
  const value = useMemo(() => ({ state, dispatch, defaults }), [defaults, dispatch, state])
  return <StateContext.Provider value={value}>{children}</StateContext.Provider>
}

export function useDashboardState() {
  const value = useContext(StateContext)
  if (!value) throw new Error('DashboardProvider missing')
  return value
}
