import { describe, expect, it } from 'vitest'
import type { Selection } from './contracts'
import { initialQuery, queryParams, readQuery, reducer } from './state'

const defaults: Selection = { period: 'week', bucket: 'day', from: '', to: '', providers: ['openai'], models: [], harnesses: [], sessions: [] }

describe('dashboard navigation', () => {
  it('restores CLI defaults only for an unconfigured URL', () => {
    expect(readQuery(new URLSearchParams(), defaults).providers).toEqual(['openai'])
    const cleared = { ...initialQuery(defaults), providers: [] }
    expect(readQuery(queryParams(cleared), defaults).providers).toEqual([])
  })

  it('round-trips multi-selects, open bounds and pagination without losing identifiers', () => {
    const q = { ...initialQuery(defaults), models: ['a/b', 'model + 1'], sessions: ['session:one'], from: '2026-01-01', page: 3 }
    expect(readQuery(queryParams(q), defaults)).toEqual(q)
  })

  it('preserves filters across tabs and resets incompatible sorting and pagination', () => {
    const state = { query: { ...initialQuery(defaults), page: 4 }, theme: 'system', hidden: [], chartMetric: 'total' } satisfies Parameters<typeof reducer>[0]
    const context = reducer(state, { type: 'tab', value: 'context' })
    expect(context.query).toMatchObject({ providers: ['openai'], page: 1, sort: 'averageContext', direction: 'desc' })
    const sorted = reducer(context, { type: 'sort', value: 'model' })
    expect(sorted.query.direction).toBe('asc')
    expect(reducer(sorted, { type: 'sort', value: 'model' }).query.direction).toBe('desc')
    expect(reducer(sorted, { type: 'selection', value: { providers: [] } }).query.page).toBe(1)
  })

  it('recovers incompatible bookmarked sorts and unsafe page sizes', () => {
    expect(readQuery(new URLSearchParams('tab=context&sort=total&pageSize=900'), defaults)).toMatchObject({ sort: 'averageContext', pageSize: 50 })
  })
})
