import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { useAnalytics } from './api'
import { createAppRouter } from './router'
import { SourceProvider } from './source-context'
import { createSource, createSourceStore } from './sources'
import { initialQuery } from './state'
import type { QueryState } from './state'
import type { Bootstrap, Dashboard, SyncStatus } from './contracts'

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

function AnalyticsExample({
  query,
  baseUrl = 'http://localhost',
}: {
  query: QueryState
  baseUrl?: string
}) {
  const data = useAnalytics(baseUrl, query, 0, true)
  return <output>{data.data ? data.data.summary.total : 'Loading'}</output>
}

function dashboard(total: number): Dashboard {
  return {
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
    apiVersion: 'v1',
    serverVersion: 'test',
    hostname: 'first-load',
    timezone: 'UTC',
    capabilities: ['usage', 'facets', 'sync'],
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
    phase: 'syncing',
    running: true,
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
  const store = createSourceStore({ origin: window.location.origin, storage: null })
  render(
    <QueryClientProvider client={client}>
      <SourceProvider store={store}>
        <RouterProvider router={testRouter} />
      </SourceProvider>
    </QueryClientProvider>,
  )

  expect(await screen.findByRole('button', { name: 'This week' })).toBeVisible()
  expect(store.getSnapshot().sources[0]?.defaults?.period).toBe('week')
  client.clear()
})

it('removes the final route filter after direct load', async () => {
  const bootstrap: Bootstrap = {
    apiVersion: 'v1',
    serverVersion: 'test',
    hostname: 'direct-load',
    timezone: 'UTC',
    capabilities: ['usage', 'facets', 'sync'],
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
    phase: 'syncing',
    running: true,
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
  const store = createSourceStore({ origin: window.location.origin, storage: null })
  const directRouter = createAppRouter(
    createMemoryHistory({
      initialEntries: [
        '/tokens?v=1&period=week&bucket=day&model=model-a&sort=date&direction=desc&page=1&pageSize=50',
      ],
    }),
  )
  render(
    <QueryClientProvider client={client}>
      <SourceProvider store={store}>
        <RouterProvider router={directRouter} />
      </SourceProvider>
    </QueryClientProvider>,
  )

  await userEvent.click(await screen.findByRole('button', { name: 'Remove model model-a' }))

  await waitFor(() => expect(directRouter.state.location.href).not.toContain('model=model-a'))
  client.clear()
})

it('keeps dashboard filters while switching sources', async () => {
  const localUrl = 'http://localhost:3000'
  const remoteUrl = 'http://remote:8765'
  const local: Bootstrap = {
    apiVersion: 'v1',
    serverVersion: 'test',
    hostname: 'local-machine',
    timezone: 'UTC',
    capabilities: ['usage', 'facets', 'sync'],
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
  const remote: Bootstrap = { ...local, hostname: 'remote-machine' }
  const status: SyncStatus = {
    phase: 'syncing',
    running: true,
    error: '',
    revision: 1,
    harnesses: {},
  }
  const store = createSourceStore({ origin: localUrl, storage: null })
  store.add(createSource(localUrl, local))
  store.add(createSource(remoteUrl, remote))
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['instance', localUrl], local)
  client.setQueryData(['instance', remoteUrl], remote)
  client.setQueryData(['sync', localUrl], status)
  client.setQueryData(['sync', remoteUrl], status)
  render(
    <QueryClientProvider client={client}>
      <SourceProvider store={store}>
        <RouterProvider router={testRouter} />
      </SourceProvider>
    </QueryClientProvider>,
  )
  const user = userEvent.setup()

  await user.click(await screen.findByRole('button', { name: 'This month' }))
  await user.click(
    within(screen.getByRole('dialog', { name: 'Date range' })).getByRole('button', {
      name: 'All time',
    }),
  )
  await user.click(screen.getByRole('button', { name: 'Choose data source' }))
  await user.click(screen.getByRole('button', { name: 'Select remote-machine source' }))

  expect(store.getSnapshot().activeUrl).toBe(remoteUrl)
  expect(
    within(screen.getByLabelText('Quick date ranges')).getByRole('button', { name: 'All time' }),
  ).toBeVisible()
  expect(testRouter.state.location.search.period).toBe('all')
  client.clear()
})

it('keeps an unavailable selected source active with recovery controls', async () => {
  const remoteUrl = 'http://offline:8765'
  const remote: Bootstrap = {
    apiVersion: 'v1',
    serverVersion: 'test',
    hostname: 'offline-machine',
    timezone: 'UTC',
    capabilities: ['usage', 'facets', 'sync'],
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
  const store = createSourceStore({ origin: 'http://localhost:3000', storage: null })
  store.add(createSource(remoteUrl, remote))
  store.select(remoteUrl)
  vi.stubGlobal('fetch', vi.fn<typeof fetch>().mockRejectedValue(new Error('Network offline')))
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <SourceProvider store={store}>
        <RouterProvider router={testRouter} />
      </SourceProvider>
    </QueryClientProvider>,
  )

  expect(
    await screen.findByRole('heading', { name: 'Couldn’t connect to offline-machine' }),
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'Choose data source' })).toBeEnabled()
  expect(store.getSnapshot().activeUrl).toBe(remoteUrl)
  client.clear()
})

it.each<{ phase: SyncStatus['phase']; running: boolean; message: string }>([
  { phase: 'resetting', running: true, message: 'Resetting usage data for compatibility…' },
  {
    phase: 'rebuilding',
    running: true,
    message: 'Rebuilding usage from all configured harnesses…',
  },
  {
    phase: 'rebuild_failed',
    running: false,
    message:
      'Usage recovery is incomplete. Retry sync with the original source configuration and database. See terminal details.',
  },
])(
  'explains $phase and prevents analytics inspection during recovery',
  async ({ phase, running, message }) => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
    const bootstrap: Bootstrap = {
      apiVersion: 'v1',
      serverVersion: 'test',
      capabilities: ['usage', 'facets', 'sync'],
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
      hostname: 'local',
      timezone: 'UTC',
    }
    const status: SyncStatus = {
      phase,
      running,
      error: running ? '' : message,
      revision: 1,
      harnesses: { codex: 'pending' },
    }
    client.setQueryData(['instance', window.location.origin], bootstrap)
    client.setQueryData(['sync', window.location.origin], status)
    const fetcher = vi.fn<typeof fetch>()
    vi.stubGlobal('fetch', fetcher)
    render(
      <QueryClientProvider client={client}>
        <SourceProvider>
          <RouterProvider router={testRouter} />
        </SourceProvider>
      </QueryClientProvider>,
    )
    expect(await screen.findByText(message)).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Inspect Existing Data' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Retry Sync' }) === null).toBe(running)
    expect(fetcher).not.toHaveBeenCalled()
    client.clear()
  },
)

it('allows explicit inspection after an ordinary sync failure', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  const bootstrap: Bootstrap = {
    apiVersion: 'v1',
    serverVersion: 'test',
    capabilities: ['usage', 'facets', 'sync'],
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
    hostname: 'local',
    timezone: 'UTC',
  }
  const status: SyncStatus = {
    phase: 'failed',
    running: false,
    error: 'Sync failed.',
    revision: 1,
    harnesses: {},
  }
  client.setQueryData(['instance', window.location.origin], bootstrap)
  client.setQueryData(['sync', window.location.origin], status)
  render(
    <QueryClientProvider client={client}>
      <SourceProvider>
        <RouterProvider router={testRouter} />
      </SourceProvider>
    </QueryClientProvider>,
  )
  expect(await screen.findByRole('button', { name: 'Inspect Existing Data' })).toBeEnabled()
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

it('isolates cached usage while switching sources', async () => {
  let resolveRemote: ((response: Response) => void) | undefined
  vi.stubGlobal(
    'fetch',
    vi.fn<(input: RequestInfo | URL) => Promise<Response>>((input) => {
      if (requestURL(input).startsWith('http://remote')) {
        return new Promise((resolve) => {
          resolveRemote = resolve
        })
      }
      return Promise.resolve(
        new Response(JSON.stringify(dashboard(111)), {
          headers: { 'Content-Type': 'application/json' },
        }),
      )
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
      <AnalyticsExample baseUrl="http://remote:8765" query={query} />
    </QueryClientProvider>,
  )
  expect(screen.getByRole('status')).toHaveTextContent('Loading')
  resolveRemote?.(
    new Response(JSON.stringify(dashboard(222)), {
      headers: { 'Content-Type': 'application/json' },
    }),
  )
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('222'))
  client.clear()
})

it('retains valid summaries across views but not filter changes', async () => {
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
