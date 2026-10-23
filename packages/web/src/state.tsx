import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { z } from 'zod'
import type { LocationGroup, Selection, Sort } from './contracts'
import { queryFromSearch, reduceQuery, searchFromQuery, stringifyDashboardSearch } from './query'
import type { DateSelection, QueryAction, QueryState } from './query'

const themeSchema = z.enum(['system', 'light', 'dark'])
export type Theme = z.infer<typeof themeSchema>

const QueryContext = createContext<{
  query: QueryState
  setQuality: (value: QueryState['quality']) => void
  defaults: Selection
  updateSelection: (value: DateSelection) => void
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
      updateSelection: (value: DateSelection) => navigateQuery({ type: 'selection', value }),
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

export function DashboardPreferencesProvider({ children }: { children: ReactNode }) {
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
