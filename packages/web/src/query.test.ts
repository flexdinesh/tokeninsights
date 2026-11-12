import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { apiQueryParams } from './http-query'
import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import {
  locationGroupSchema,
  selectionSchema,
  sortSchema,
  tabSchema,
} from '@tokeninsights/contracts'
import type { Selection } from '@tokeninsights/contracts'
import {
  defaultSort,
  initialQuery,
  parseDashboardSearch,
  parseDashboardSearchParams,
  queryFromSearch,
  reduceQuery,
  searchFromQuery,
  stringifyDashboardSearch,
} from '../../dashboard/src/query'
import type { QueryAction, QueryState } from '../../dashboard/src/query'

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
  it('preserves calendar navigation through URL, view, and filter changes without changing the API', () => {
    const selected = reduceQuery(initialQuery(defaults), {
      type: 'selection',
      value: { period: 'month', from: '2026-09-01', to: '2026-09-30', dateMode: 'calendar' },
    })
    const filtered = reduceQuery(selected, { type: 'selection', value: { models: ['model-a'] } })
    const models = reduceQuery(filtered, { type: 'tab', value: 'models' })
    const restored = queryFromSearch(
      parseDashboardSearchParams(stringifyDashboardSearch(searchFromQuery(models))),
      'models',
      defaults,
    )
    expect(restored).toMatchObject({
      dateMode: 'calendar',
      from: '2026-09-01',
      to: '2026-09-30',
      models: ['model-a'],
      page: 1,
    })
    expect(apiQueryParams(restored).get('from')).toBe('2026-09-01')
    expect(apiQueryParams(restored).has('dateMode')).toBe(false)
    expect(reduceQuery(restored, { type: 'clearFilters' })).toMatchObject({
      from: '',
      to: '',
      dateMode: undefined,
    })
  })

  it('keeps existing explicit-bound links custom and drops calendar mode without both bounds', () => {
    expect(
      queryFromSearch(
        parseDashboardSearchParams('?period=week&from=2026-10-01'),
        'tokens',
        defaults,
      ).dateMode,
    ).toBeUndefined()
    expect(
      queryFromSearch(
        parseDashboardSearchParams('?period=week&dateMode=calendar&from=2026-10-01'),
        'tokens',
        defaults,
      ).dateMode,
    ).toBeUndefined()
  })

  it.each([
    ['2026-13-01', '2026-13-07'],
    ['2026-10-18', '2026-10-12'],
    ['2026-10-12', '2026-10-15'],
  ])('treats invalid or partial calendar bounds as custom (%s to %s)', (from, to) => {
    const query = queryFromSearch(
      parseDashboardSearch({ period: 'week', dateMode: 'calendar', from, to }),
      'tokens',
      defaults,
    )
    expect(query.dateMode).toBeUndefined()
    expect(query).toMatchObject({ from, to })
  })
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

  it('recovers incompatible bookmarked sorts and unsafe page sizes', () => {
    const search = parseDashboardSearch({ v: 1, sort: 'total', pageSize: 900 })
    expect(queryFromSearch(search, 'context', defaults)).toMatchObject({
      sort: 'averageContext',
      pageSize: 50,
    })
  })

  it('round trips Repo options and clears location filters on other views', () => {
    const repo: QueryState = {
      ...initialQuery(defaults, 'repo'),
      locationGroup: 'directory',
      repositories: ['repo-key'],
      directories: ['dir-key'],
    }
    const value = stringifyDashboardSearch(searchFromQuery(repo))
    expect(value).toContain('locationGroup=directory')
    expect(value).toContain('repository=repo-key')
    expect(queryFromSearch(parseDashboardSearchParams(value), 'repo', defaults)).toMatchObject(repo)
    const tokens = reduceQuery(repo, { type: 'tab', value: 'tokens' })
    expect(tokens.repositories).toEqual([])
    expect(stringifyDashboardSearch(searchFromQuery(tokens))).not.toContain('repository=')
  })

  it('drops removed Repo options from saved URLs', () => {
    const search = parseDashboardSearchParams(
      '?v=1&locationGroup=worktree&worktree=old&branch=main&breakdown=model&repository=repo-key',
    )
    const query = queryFromSearch(search, 'repo', defaults)
    expect(query.locationGroup).toBe('repository')
    expect(query.repositories).toEqual(['repo-key'])
    expect(stringifyDashboardSearch(searchFromQuery(query))).not.toMatch(
      /worktree|branch|breakdown/,
    )
  })
})

// Shared with packages/core/dashboardview so native query state stays identical.
const quality = z.enum(['confirmed', 'estimated'])
const strings = z.array(z.string())
const queryState = selectionSchema.extend({
  dateMode: z.literal('calendar').optional(),
  quality,
  tab: tabSchema,
  locationGroup: locationGroupSchema,
  repositories: strings,
  directories: strings,
  sort: sortSchema,
  direction: z.enum(['asc', 'desc']),
  page: z.int().min(1),
  pageSize: z.int().min(1),
})
const action = z.discriminatedUnion('type', [
  z.strictObject({ type: z.literal('quality'), value: quality }),
  z.strictObject({
    type: z.literal('selection'),
    // null is an explicit clear; an absent dateMode keeps calendar navigation.
    value: selectionSchema
      .partial()
      .extend({ dateMode: z.literal('calendar').nullable().optional() }),
  }),
  z.strictObject({ type: z.literal('reset'), value: selectionSchema }),
  z.strictObject({ type: z.literal('clearFilters') }),
  z.strictObject({ type: z.literal('tab'), value: tabSchema }),
  z.strictObject({ type: z.literal('locationGroup'), value: locationGroupSchema }),
  z.strictObject({
    type: z.literal('locations'),
    value: z.strictObject({ repositories: strings.optional(), directories: strings.optional() }),
  }),
  z.strictObject({ type: z.literal('sort'), value: sortSchema }),
  z.strictObject({ type: z.literal('page'), value: z.int().min(1) }),
  z.strictObject({ type: z.literal('pageSize'), value: z.int().min(1) }),
])
const fixture = z
  .strictObject({
    defaults: selectionSchema,
    defaultSort: z.record(tabSchema, sortSchema),
    compatibleSort: z.array(
      z.strictObject({ tab: tabSchema, sort: sortSchema, expected: sortSchema }),
    ),
    reducer: z.array(
      z.strictObject({
        name: z.string(),
        tab: tabSchema,
        actions: z.array(action),
        expected: queryState,
        apiParams: z.array(z.tuple([z.string(), z.string()])).optional(),
      }),
    ),
  })
  .parse(
    JSON.parse(
      readFileSync(
        join(import.meta.dirname, '../../contracts/fixtures/dashboard-view/query.json'),
        'utf8',
      ),
    ),
  )

function queryAction(value: z.infer<typeof action>): QueryAction {
  if (value.type !== 'selection') return value
  const { dateMode, ...selection } = value.value
  return {
    type: 'selection',
    value: dateMode === undefined ? selection : { ...selection, dateMode: dateMode ?? undefined },
  }
}

describe('shared query fixtures', () => {
  it('starts each view on its default sort', () => {
    for (const tab of tabSchema.options) expect(defaultSort(tab)).toBe(fixture.defaultSort[tab])
  })

  it('recovers sorts a restored view cannot use', () => {
    for (const { tab, sort, expected } of fixture.compatibleSort) {
      const search = parseDashboardSearch({ v: 1, sort })
      expect(queryFromSearch(search, tab, fixture.defaults).sort, `${tab} ${sort}`).toBe(expected)
    }
  })

  function reduced(tab: QueryState['tab'], actions: z.infer<typeof action>[]): QueryState {
    return actions.reduce(
      (state, value) => reduceQuery(state, queryAction(value)),
      initialQuery(fixture.defaults, tab),
    )
  }

  it.each(fixture.reducer)('$name', ({ tab, actions, expected }) => {
    expect(reduced(tab, actions)).toEqual(expected)
  })

  const apiCases = fixture.reducer.flatMap(({ name, tab, actions, apiParams }) =>
    apiParams ? [{ name, tab, actions, apiParams }] : [],
  )
  it.each(apiCases)('requests $name', ({ tab, actions, apiParams }) => {
    expect([...apiQueryParams(reduced(tab, actions))]).toEqual(apiParams)
  })
})
