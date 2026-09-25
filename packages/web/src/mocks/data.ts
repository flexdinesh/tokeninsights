import {
  bootstrapSchema,
  dashboardSchema,
  facetsSchema,
  rowSchema,
  statusSchema,
  tabSchema,
} from '../contracts'
import type { Bootstrap, Dashboard, Facets, Row, SyncStatus, Tab } from '../contracts'

const dayOne = Date.UTC(2026, 8, 12, 10)
const dayTwo = Date.UTC(2026, 8, 13, 10)
const dayThree = Date.UTC(2026, 8, 14, 10)

export const mockBootstrap: Bootstrap = bootstrapSchema.parse({
  apiVersion: 'v1',
  serverVersion: 'dev-mock',
  hostname: 'mock.tokeninsights.local',
  timezone: 'AEST +10:00',
  capabilities: ['usage', 'facets', 'sync'],
  defaults: {
    period: 'all',
    bucket: 'day',
    from: '',
    to: '',
    providers: [],
    models: [],
    harnesses: [],
    sessions: [],
  },
})

export const mockFacets: Facets = facetsSchema.parse({
  providers: ['anthropic', 'openai'],
  models: ['claude-sonnet-4-5', 'gpt-5', 'gpt-5-mini'],
  harnesses: ['claude-code', 'codex', 'opencode', 'pi'],
  sessions: [
    'mock-claude-session',
    'mock-codex-session',
    'mock-opencode-session',
    'mock-pi-session',
  ],
  repositories: [
    { key: 'repo-1', name: 'tokeninsights' },
    { key: 'unknown', name: 'unknown' },
  ],
  directories: [
    { key: 'directory-1', name: '~/workspace/tokeninsights/main' },
    { key: 'directory-2', name: '~/workspace/client-a' },
    { key: 'directory-3', name: '~/workspace/client-b' },
    { key: 'directory-4', name: '~/workspace/scratch' },
    { key: 'directory-5', name: '~/workspace/tools' },
    { key: 'unknown', name: 'unknown' },
  ],
})

function row(values: Partial<Row>): Row {
  return rowSchema.parse({
    key: '',
    name: '',
    harness: '',
    provider: '',
    model: '',
    date: 0,
    sessions: 0,
    input: 0,
    output: 0,
    reasoning: 0,
    cacheRead: 0,
    cacheWrite: 0,
    total: 0,
    context: 0,
    averageContext: 0,
    medianContext: 0,
    maxContext: 0,
    locationKey: '',
    locationName: '',
    directoryNames: [],
    hasUnknownDirectory: false,
    repositoryKey: '',
    repositoryName: '',
    ...values,
  })
}

const tokenRows = [
  row({
    key: '2026-09-12',
    name: '2026-09-12',
    date: dayOne,
    sessions: 3,
    input: 126_000,
    output: 25_000,
    reasoning: 4_000,
    cacheRead: 62_000,
    cacheWrite: 3_000,
    total: 220_000,
    context: 191_000,
  }),
  row({
    key: '2026-09-13',
    name: '2026-09-13',
    date: dayTwo,
    sessions: 4,
    input: 145_000,
    output: 31_000,
    reasoning: 6_000,
    cacheRead: 71_000,
    cacheWrite: 4_000,
    total: 257_000,
    context: 220_000,
  }),
  row({
    key: '2026-09-14',
    name: '2026-09-14',
    date: dayThree,
    sessions: 3,
    input: 98_000,
    output: 22_000,
    reasoning: 3_000,
    cacheRead: 45_000,
    cacheWrite: 2_000,
    total: 170_000,
    context: 145_000,
  }),
]

const rowsByTab: Record<Tab, Row[]> = {
  tokens: tokenRows,
  models: [
    row({ key: 'gpt-5', name: 'gpt-5', sessions: 2, total: 311_000 }),
    row({ key: 'claude-sonnet-4-5', name: 'claude-sonnet-4-5', sessions: 1, total: 205_000 }),
    row({ key: 'gpt-5-mini', name: 'gpt-5-mini', sessions: 1, total: 131_000 }),
  ],
  providers: [
    row({ key: 'openai', name: 'openai', sessions: 3, total: 442_000 }),
    row({ key: 'anthropic', name: 'anthropic', sessions: 1, total: 205_000 }),
  ],
  harnesses: [
    row({ key: 'codex', name: 'codex', sessions: 1, total: 244_000 }),
    row({ key: 'claude-code', name: 'claude-code', sessions: 1, total: 181_000 }),
    row({ key: 'opencode', name: 'opencode', sessions: 1, total: 135_000 }),
    row({ key: 'pi', name: 'pi', sessions: 1, total: 87_000 }),
  ],
  sessions: [
    row({
      key: 'codex:mock-codex-session',
      name: 'mock-codex-session',
      harness: 'codex',
      provider: 'openai',
      model: 'gpt-5',
      date: dayThree,
      sessions: 1,
      total: 244_000,
    }),
    row({
      key: 'claude-code:mock-claude-session',
      name: 'mock-claude-session',
      harness: 'claude-code',
      provider: 'anthropic',
      model: 'claude-sonnet-4-5',
      date: dayThree,
      sessions: 1,
      total: 181_000,
    }),
    row({
      key: 'opencode:mock-opencode-session',
      name: 'mock-opencode-session',
      harness: 'opencode',
      provider: 'openai',
      model: 'gpt-5-mini',
      date: dayTwo,
      sessions: 1,
      total: 135_000,
    }),
    row({
      key: 'pi:mock-pi-session',
      name: 'mock-pi-session',
      harness: 'pi',
      provider: 'openai',
      model: 'gpt-5',
      date: dayOne,
      sessions: 1,
      total: 87_000,
    }),
  ],
  context: [
    row({
      key: 'codex\u0000openai\u0000gpt-5',
      name: 'gpt-5',
      harness: 'codex',
      provider: 'openai',
      model: 'gpt-5',
      sessions: 1,
      averageContext: 118_000,
      medianContext: 121_000,
      maxContext: 184_000,
    }),
    row({
      key: 'claude-code\u0000anthropic\u0000claude-sonnet-4-5',
      name: 'claude-sonnet-4-5',
      harness: 'claude-code',
      provider: 'anthropic',
      model: 'claude-sonnet-4-5',
      sessions: 1,
      averageContext: 92_000,
      medianContext: 92_000,
      maxContext: 133_000,
    }),
    row({
      key: 'opencode\u0000openai\u0000gpt-5-mini',
      name: 'gpt-5-mini',
      harness: 'opencode',
      provider: 'openai',
      model: 'gpt-5-mini',
      sessions: 1,
      averageContext: 61_000,
      medianContext: 61_000,
      maxContext: 89_000,
    }),
  ],
  repo: [
    row({
      key: 'repo-1\u0000',
      name: 'tokeninsights',
      locationKey: 'repo-1',
      locationName: 'tokeninsights',
      repositoryKey: 'repo-1',
      repositoryName: 'tokeninsights',
      provider: 'anthropic, openai',
      harness: 'codex, pi',
      model: 'claude-sonnet-4-5, gpt-5',
      sessions: 3,
      total: 560_000,
    }),
    row({
      key: 'unknown\u0000',
      name: 'unknown',
      locationKey: 'unknown',
      locationName: 'unknown',
      repositoryKey: 'unknown',
      repositoryName: 'unknown',
      directoryNames: [
        '~/workspace/client-a',
        '~/workspace/client-b',
        '~/workspace/scratch',
        '~/workspace/tools',
      ],
      hasUnknownDirectory: true,
      provider: 'openai',
      harness: 'opencode',
      model: 'gpt-5-mini',
      sessions: 1,
      total: 87_000,
    }),
  ],
}

export const mockTabs: Tab[] = [
  'tokens',
  'models',
  'providers',
  'harnesses',
  'sessions',
  'context',
  'repo',
]

function positiveInteger(value: string | null, fallback: number, maximum: number): number {
  if (value === null) return fallback
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) && parsed > 0 && parsed <= maximum ? parsed : fallback
}

export function mockDashboard(
  tabValue: string | null,
  pageValue: string | null = null,
  pageSizeValue: string | null = null,
): Dashboard {
  const parsedTab = tabSchema.safeParse(tabValue ?? 'tokens')
  const tab = parsedTab.success ? parsedTab.data : 'tokens'
  const allRows = rowsByTab[tab]
  const pageSize = positiveInteger(pageSizeValue, 50, 200)
  const requestedPage = positiveInteger(pageValue, 1, Number.MAX_SAFE_INTEGER)
  const pageCount = Math.max(1, Math.ceil(allRows.length / pageSize))
  const page = Math.min(requestedPage, pageCount)
  const start = (page - 1) * pageSize

  return dashboardSchema.parse({
    rows: allRows.slice(start, start + pageSize),
    chart: allRows,
    rowCount: allRows.length,
    page,
    pageSize,
    summary: {
      total: 647_000,
      input: 369_000,
      output: 78_000,
      reasoning: 13_000,
      cacheRead: 178_000,
      cacheWrite: 9_000,
      sessions: 4,
      syncedSessions: 5,
    },
    lastSynced: dayThree,
    range: 'Sep 12 – Sep 14, 2026',
  })
}

export function mockSyncStatus(running: boolean, revision: number): SyncStatus {
  return statusSchema.parse({
    running,
    phase: running ? 'syncing' : 'ready',
    harnesses: running
      ? { opencode: 'synced', pi: 'syncing', codex: 'pending', 'claude-code': 'pending' }
      : {},
    error: '',
    revision,
  })
}
