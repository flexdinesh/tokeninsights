import { createContext, useContext, useEffect, useReducer, useRef } from 'react'
import type { Dispatch, ReactNode } from 'react'
import { z } from 'zod'
import { bucketSchema, periodSchema, sortSchema, tabSchema } from './contracts'
import type { Selection, Sort, Tab } from './contracts'

export interface QueryState extends Selection { tab: Tab; sort: Sort; direction: 'asc' | 'desc'; page: number; pageSize: number }
const themeSchema = z.enum(['system', 'light', 'dark'])
export type Theme = z.infer<typeof themeSchema>
interface State { query: QueryState; theme: Theme; hidden: string[]; chartMetric: Sort }
type Action =
  | { type: 'selection'; value: Partial<Selection> }
  | { type: 'tab'; value: Tab }
  | { type: 'sort'; value: Sort }
  | { type: 'page'; value: number }
  | { type: 'pageSize'; value: number }
  | { type: 'replace'; value: QueryState }
  | { type: 'theme'; value: Theme }
  | { type: 'column'; value: string }
  | { type: 'chartMetric'; value: Sort }

export function defaultSort(tab: Tab): Sort { return tab === 'context' ? 'averageContext' : tab === 'tokens' || tab === 'sessions' ? 'date' : 'total' }
export function initialQuery(defaults: Selection): QueryState { return { ...defaults, tab: 'tokens', sort: 'date', direction: 'desc', page: 1, pageSize: 50 } }

export function reducer(state: State, action: Action): State {
  const q = state.query
  switch (action.type) {
    case 'selection': return { ...state, query: { ...q, ...action.value, page: 1 } }
    case 'tab': return { ...state, query: { ...q, tab: action.value, sort: defaultSort(action.value), direction: 'desc', page: 1 } }
    case 'sort': return { ...state, query: { ...q, sort: action.value, direction: q.sort === action.value ? (q.direction === 'asc' ? 'desc' : 'asc') : ['name', 'harness', 'provider', 'model'].includes(action.value) ? 'asc' : 'desc', page: 1 } }
    case 'page': return { ...state, query: { ...q, page: action.value } }
    case 'pageSize': return { ...state, query: { ...q, pageSize: action.value, page: 1 } }
    case 'replace': return { ...state, query: action.value }
    case 'theme': return { ...state, theme: action.value }
    case 'column': return { ...state, hidden: state.hidden.includes(action.value) ? state.hidden.filter(v => v !== action.value) : [...state.hidden, action.value] }
    case 'chartMetric': return { ...state, chartMetric: action.value }
  }
}

export function queryParams(q: QueryState): URLSearchParams {
  const p = new URLSearchParams({ v: '1', period: q.period, bucket: q.bucket, tab: q.tab, sort: q.sort, direction: q.direction, page: String(q.page), pageSize: String(q.pageSize) })
  if (q.from) p.set('from', q.from)
  if (q.to) p.set('to', q.to)
  for (const value of q.providers) p.append('provider', value)
  for (const value of q.models) p.append('model', value)
  for (const value of q.harnesses) p.append('harness', value)
  for (const value of q.sessions) p.append('session', value)
  return p
}

export function readQuery(p: URLSearchParams, defaults: Selection): QueryState {
  if (p.size === 0) return initialQuery(defaults)
  const tab = tabSchema.catch('tokens').parse(p.get('tab'))
  let sort = sortSchema.catch(defaultSort(tab)).parse(p.get('sort'))
  const contextSorts: Sort[] = ['averageContext', 'medianContext', 'maxContext', 'sessions', 'harness', 'provider', 'model']
  if (tab === 'context' ? !contextSorts.includes(sort) : contextSorts.some(v => v !== 'sessions' && v === sort)) sort = defaultSort(tab)
  const date = (key: string) => { const v = p.get(key) ?? ''; return /^\d{4}-\d{2}-\d{2}$/.test(v) ? v : '' }
  const positive = (key: string, fallback: number, max: number) => { const n = Number(p.get(key)); return Number.isSafeInteger(n) && n > 0 && n <= max ? n : fallback }
  return {
    period: periodSchema.catch(defaults.period).parse(p.get('period')), bucket: bucketSchema.catch(defaults.bucket).parse(p.get('bucket')),
    from: date('from'), to: date('to'), providers: p.getAll('provider'), models: p.getAll('model'),
    harnesses: p.getAll('harness'), sessions: p.getAll('session'), tab, sort,
    direction: p.get('direction') === 'asc' ? 'asc' : 'desc', page: positive('page', 1, Number.MAX_SAFE_INTEGER), pageSize: positive('pageSize', 50, 200),
  }
}

const StateContext = createContext<{ state: State; dispatch: Dispatch<Action>; defaults: Selection } | null>(null)

function savedTheme(): Theme {
  try { return themeSchema.catch('system').parse(localStorage.getItem('tokeninsights.theme')) } catch { return 'system' }
}

export function DashboardProvider({ defaults, children }: { defaults: Selection; children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, { query: readQuery(new URLSearchParams(location.search), defaults), theme: savedTheme(), hidden: [], chartMetric: 'total' })
  const mounted = useRef(false)
  useEffect(() => {
    const search = `?${queryParams(state.query)}`
    if (location.search !== search) {
      if (mounted.current) history.pushState(null, '', search)
      else history.replaceState(null, '', search)
    }
    mounted.current = true
  }, [state.query])
  useEffect(() => {
    const onPop = () => dispatch({ type: 'replace', value: readQuery(new URLSearchParams(location.search), defaults) })
    addEventListener('popstate', onPop)
    return () => removeEventListener('popstate', onPop)
  }, [defaults])
  useEffect(() => {
    document.documentElement.dataset.theme = state.theme
    try { localStorage.setItem('tokeninsights.theme', state.theme) } catch { /* Preferences still work without storage. */ }
  }, [state.theme])
  return <StateContext.Provider value={{ state, dispatch, defaults }}>{children}</StateContext.Provider>
}

export function useDashboardState() {
  const value = useContext(StateContext)
  if (!value) throw new Error('DashboardProvider missing')
  return value
}
