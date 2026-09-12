import { describe, expect, it } from 'vitest'
import type { Selection } from './contracts'
import {
  initialQuery,
  parseDashboardSearch,
  parseDashboardSearchParams,
  queryFromSearch,
  reduceQuery,
  searchFromQuery,
  stringifyDashboardSearch,
} from './state'

const defaults: Selection = {
  period: 'week',
  bucket: 'day',
  from: '',
  to: '',
  providers: ['openai'],
  models: [],
  harnesses: [],
  sessions: [],
}

describe('dashboard navigation', () => {
  it('restores CLI defaults only for an unconfigured route', () => {
    expect(queryFromSearch(parseDashboardSearch({}), 'tokens', defaults).providers).toEqual([
      'openai',
    ])
    const cleared = { ...initialQuery(defaults), providers: [] }
    expect(queryFromSearch(searchFromQuery(cleared), 'tokens', defaults).providers).toEqual([])
  })

  it('serializes repeated filters and open bounds without losing identifiers', () => {
    const query = {
      ...initialQuery(defaults),
      models: ['a/b', 'model + 1'],
      sessions: ['session:one'],
      from: '2026-01-01',
      page: 3,
    }
    const value = stringifyDashboardSearch(searchFromQuery(query))
    expect(value).toContain('model=a%2Fb&model=model+%2B+1')
    expect(value).toContain('session=session%3Aone')
    expect(value).toContain('from=2026-01-01')
    expect(value).not.toContain('tab=')
    expect(parseDashboardSearchParams(value).models).toEqual(['a/b', 'model + 1'])
  })

  it('preserves filters across paths and resets incompatible sorting and pagination', () => {
    const query = { ...initialQuery(defaults), page: 4 }
    const context = reduceQuery(query, { type: 'tab', value: 'context' })
    expect(context).toMatchObject({
      providers: ['openai'],
      tab: 'context',
      page: 1,
      sort: 'averageContext',
      direction: 'desc',
    })
    const sorted = reduceQuery(context, { type: 'sort', value: 'model' })
    expect(sorted.direction).toBe('asc')
    expect(reduceQuery(sorted, { type: 'sort', value: 'model' }).direction).toBe('desc')
    expect(reduceQuery(sorted, { type: 'selection', value: { providers: [] } }).page).toBe(1)
  })

  it('recovers incompatible bookmarked sorts and unsafe page sizes', () => {
    const search = parseDashboardSearch({ v: 1, sort: 'total', pageSize: 900 })
    expect(queryFromSearch(search, 'context', defaults)).toMatchObject({
      sort: 'averageContext',
      pageSize: 50,
    })
  })

  it('recognizes legacy tab query parameters for redirect', () => {
    expect(parseDashboardSearch({ tab: 'models' }).legacyTab).toBe('models')
    expect(parseDashboardSearchParams('?tab=providers').legacyTab).toBe('providers')
    expect(parseDashboardSearch({ tab: 'invalid' }).legacyTab).toBeUndefined()
  })
})
