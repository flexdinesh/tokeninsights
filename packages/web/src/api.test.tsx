import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { useAnalytics } from './api'
import { createAppRouter } from './router'
import { initialQuery } from './state'
import type { QueryState } from './state'
import type { Bootstrap, Dashboard, SyncStatus } from './contracts'

const testIdentity = { instanceId: 'test-instance', dataEpoch: 'test-database' }

let testRouter = createAppRouter(createMemoryHistory({ initialEntries: ['/tokens'] }))

beforeEach(() => {
  testRouter = createAppRouter(createMemoryHistory({ initialEntries: ['/tokens'] }))
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  window.localStorage?.clear()
})

function requestURL(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  return input instanceof URL ? input.href : input.url
}

function AnalyticsExample({ query }: { query: QueryState }) {
  const data = useAnalytics(query, 1, true, 'test-instance/test-database')
  return (
    <output data-tab={data.data?.tab} data-placeholder={data.isPlaceholderData}>
      {data.data ? data.data.dashboard.summary.total : 'Loading'}
    </output>
  )
}

function dashboard(total: number): Dashboard {
  return {
    ...testIdentity,
    revision: 1,
    rows: [],
    chart: [],
    rowCount: 0,
    page: 1,
    pageSize: 50,
    range: 'week',
    lastSynced: 0,
    summary: {
      total,
      input: total,
      output: 0,
      cacheRead: 0,
      cacheWrite: 0,
      reasoning: 0,
      sessions: 1,
      syncedSessions: 1,
    },
  }
}

it('uses server defaults on first load', async () => {
  const bootstrap: Bootstrap = {
    ...testIdentity,
    dataReadiness: 'ready',
    apiVersion: 'v1',
    serverVersion: 'test',
    hostname: 'first-load',
    timezone: 'UTC',
    capabilities: ['usage', 'facets', 'ingestion'],
    defaults: {
      period: 'week',
      bucket: 'day',
      from: '',
      to: '',
      providers: [],
      models: [],
      harnesses: [],
      sessions: [],
    },
  }
  const status: SyncStatus = {
    ...testIdentity,
    dataReadiness: 'ready',
    phase: 'ready',
    running: false,
    error: '',
    revision: 1,
    harnesses: {},
  }
  vi.stubGlobal(
    'fetch',
    vi.fn<(input: RequestInfo | URL) => Promise<Response>>((input) =>
      Promise.resolve(
        new Response(
          JSON.stringify(requestURL(input).endsWith('/api/v1/instance') ? bootstrap : status),
          { headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    ),
  )
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={testRouter} />
    </QueryClientProvider>,
  )

  expect(await screen.findByRole('button', { name: 'This week' })).toBeVisible()
  client.clear()
})

it('shows saved ingested usage without a collector', async () => {
  const bootstrap: Bootstrap = {
    ...testIdentity,
    dataReadiness: 'ready',
    apiVersion: 'v1',
    serverVersion: 'test',
    hostname: 'local',
    timezone: 'UTC',
    capabilities: ['usage', 'facets', 'ingestion'],
    defaults: {
      period: 'month',
      bucket: 'day',
      from: '',
      to: '',
      providers: [],
      models: [],
      harnesses: [],
      sessions: [],
    },
  }
  const status: SyncStatus = {
    ...testIdentity,
    dataReadiness: 'ready',
    phase: 'ready',
    running: false,
    error: '',
    revision: 1,
    harnesses: {},
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['instance'], bootstrap)
  client.setQueryData(['sync'], status)
  const fetcher = vi.fn<(input: RequestInfo | URL) => Promise<Response>>((input) => {
    const path = requestURL(input)
    const body = path.includes('/usage/facets')
      ? {
          ...testIdentity,
          revision: 1,
          providers: [],
          models: [],
          harnesses: [],
          sessions: [],
          repositories: [],
          directories: [],
        }
      : path.includes('/usage?')
        ? { ...dashboard(123), lastSynced: 1000 }
        : status
    return Promise.resolve(
      new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } }),
    )
  })
  vi.stubGlobal('fetch', fetcher)
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={testRouter} />
    </QueryClientProvider>,
  )

  await waitFor(() =>
    expect(fetcher.mock.calls.some(([input]) => requestURL(input).includes('/api/v1/usage?'))).toBe(
      true,
    ),
  )
  expect(await screen.findByLabelText('Total tokens: 123')).toBeVisible()
  client.clear()
})

it('removes the final route filter after direct load', async () => {
  const bootstrap: Bootstrap = {
    ...testIdentity,
    dataReadiness: 'ready',
    apiVersion: 'v1',
    serverVersion: 'test',
    hostname: 'direct-load',
    timezone: 'UTC',
    capabilities: ['usage', 'facets', 'ingestion'],
    defaults: {
      period: 'week',
      bucket: 'day',
      from: '',
      to: '',
      providers: [],
      models: [],
      harnesses: [],
      sessions: [],
    },
  }
  const status: SyncStatus = {
    ...testIdentity,
    dataReadiness: 'ready',
    phase: 'ready',
    running: false,
    error: '',
    revision: 1,
    harnesses: {},
  }
  vi.stubGlobal(
    'fetch',
    vi.fn<(input: RequestInfo | URL) => Promise<Response>>((input) =>
      Promise.resolve(
        new Response(
          JSON.stringify(requestURL(input).endsWith('/api/v1/instance') ? bootstrap : status),
          { headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    ),
  )
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const directRouter = createAppRouter(
    createMemoryHistory({
      initialEntries: [
        '/tokens?v=1&period=week&bucket=day&model=model-a&sort=date&direction=desc&page=1&pageSize=50',
      ],
    }),
  )
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={directRouter} />
    </QueryClientProvider>,
  )

  await userEvent.click(await screen.findByRole('button', { name: 'Remove model model-a' }))

  await waitFor(() => expect(directRouter.state.location.href).not.toContain('model=model-a'))
  client.clear()
})

it('refreshes producer labels when ingestion commits a new revision', async () => {
  const bootstrap: Bootstrap = {
    ...testIdentity,
    dataReadiness: 'ready',
    apiVersion: 'v1',
    serverVersion: 'test',
    hostname: 'unknown',
    timezone: 'UTC',
    capabilities: ['usage', 'facets', 'ingestion'],
    defaults: {
      period: 'week',
      bucket: 'day',
      from: '',
      to: '',
      providers: [],
      models: [],
      harnesses: [],
      sessions: [],
    },
  }
  const status: SyncStatus = {
    ...testIdentity,
    dataReadiness: 'ready',
    phase: 'ready',
    running: false,
    error: '',
    revision: 1,
    harnesses: {},
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['instance'], bootstrap)
  client.setQueryData(['sync'], status)
  vi.stubGlobal(
    'fetch',
    vi.fn<(input: RequestInfo | URL) => Promise<Response>>((input) => {
      const path = requestURL(input)
      const body = path.endsWith('/instance')
        ? { ...bootstrap, hostname: 'collector-workstation' }
        : path.includes('/usage/facets')
          ? {
              ...testIdentity,
              revision: 1,
              providers: [],
              models: [],
              harnesses: [],
              sessions: [],
              repositories: [],
              directories: [],
            }
          : dashboard(123)
      return Promise.resolve(
        new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } }),
      )
    }),
  )
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={testRouter} />
    </QueryClientProvider>,
  )
  const header = await screen.findByRole('banner')
  expect(within(header).getByText('unknown')).toBeVisible()
  await act(async () => {
    client.setQueryData(['sync'], { ...status, revision: 2 })
  })
  expect(await within(header).findByText('collector-workstation')).toBeVisible()
  client.clear()
})

it('offers retry when the page server is unavailable', async () => {
  const fetcher = vi.fn<typeof fetch>().mockRejectedValue(new Error('Network offline'))
  vi.stubGlobal('fetch', fetcher)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={testRouter} />
    </QueryClientProvider>,
  )
  expect(
    await screen.findByRole('heading', { name: `Couldn’t connect to ${window.location.host}` }),
  ).toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: 'Retry Connection' }))
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2))
  expect(fetcher.mock.calls.every(([input]) => requestURL(input) === '/api/v1/instance')).toBe(true)
  client.clear()
})

it('cancels obsolete filter requests and only renders the current result', async () => {
  let aborted = false
  const fetcher = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(
    (input, init) => {
      if (requestURL(input).includes('model=old'))
        return new Promise((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => {
            aborted = true
            reject(new DOMException('Aborted', 'AbortError'))
          })
        })
      const data: Dashboard = {
        ...testIdentity,
        revision: 1,
        rows: [],
        chart: [],
        rowCount: 0,
        page: 1,
        pageSize: 50,
        range: 'week',
        lastSynced: 0,
        summary: {
          total: 123,
          input: 100,
          output: 23,
          cacheRead: 0,
          cacheWrite: 0,
          reasoning: 0,
          sessions: 1,
          syncedSessions: 10,
        },
      }
      return Promise.resolve(
        new Response(JSON.stringify(data), { headers: { 'Content-Type': 'application/json' } }),
      )
    },
  )
  vi.stubGlobal('fetch', fetcher)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const base = initialQuery({
    period: 'week',
    bucket: 'day',
    from: '',
    to: '',
    providers: [],
    models: ['old'],
    harnesses: [],
    sessions: [],
  })
  const view = render(
    <QueryClientProvider client={client}>
      <AnalyticsExample query={base} />
    </QueryClientProvider>,
  )
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1))
  view.rerender(
    <QueryClientProvider client={client}>
      <AnalyticsExample query={{ ...base, models: ['new'] }} />
    </QueryClientProvider>,
  )
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('123'))
  expect(aborted).toBe(true)
  client.clear()
})

it('retains current results for sorting and valid summaries across views', async () => {
  let requests = 0
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(() => {
      requests++
      if (requests === 1) {
        return Promise.resolve(
          new Response(JSON.stringify(dashboard(111)), {
            headers: { 'Content-Type': 'application/json' },
          }),
        )
      }
      return new Promise<Response>(() => undefined)
    }),
  )
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const query = initialQuery({
    period: 'week',
    bucket: 'day',
    from: '',
    to: '',
    providers: [],
    models: [],
    harnesses: [],
    sessions: [],
  })
  const view = render(
    <QueryClientProvider client={client}>
      <AnalyticsExample query={query} />
    </QueryClientProvider>,
  )
  expect(await screen.findByText('111')).toBeVisible()

  view.rerender(
    <QueryClientProvider client={client}>
      <AnalyticsExample query={{ ...query, sort: 'total' }} />
    </QueryClientProvider>,
  )
  expect(screen.getByRole('status')).toHaveAttribute('data-tab', 'tokens')
  expect(screen.getByRole('status')).toHaveAttribute('data-placeholder', 'true')

  view.rerender(
    <QueryClientProvider client={client}>
      <AnalyticsExample query={{ ...query, tab: 'models', sort: 'total' }} />
    </QueryClientProvider>,
  )
  expect(screen.getByRole('status')).toHaveTextContent('111')

  view.rerender(
    <QueryClientProvider client={client}>
      <AnalyticsExample query={{ ...query, tab: 'models', sort: 'total', models: ['different'] }} />
    </QueryClientProvider>,
  )
  expect(screen.getByRole('status')).toHaveTextContent('Loading')
  client.clear()
})
