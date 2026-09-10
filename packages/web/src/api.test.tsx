import { afterEach, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useAnalytics } from './api'
import { App } from './App'
import { initialQuery } from './state'
import type { QueryState } from './state'
import type { Bootstrap, Dashboard, SyncStatus } from './contracts'

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

it.each([
  { phase: 'resetting', running: true, message: 'Resetting local usage data for compatibility…' },
  { phase: 'rebuilding', running: true, message: 'Rebuilding usage from all configured local harnesses…' },
  { phase: 'rebuild_failed', running: false, message: 'Usage recovery is incomplete. Retry sync with the original source configuration and database. See terminal details.' },
])('explains $phase and prevents analytics inspection during recovery', async ({ phase, running, message }) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  const bootstrap: Bootstrap = { defaults: { period: 'month', bucket: 'day', from: '', to: '', providers: [], models: [], harnesses: [], sessions: [] }, hostname: 'local', timezone: 'UTC' }
  const status: SyncStatus = { phase, running, error: running ? '' : message, revision: 1, harnesses: { codex: 'pending' } }
  client.setQueryData(['bootstrap'], bootstrap)
  client.setQueryData(['status'], status)
  const fetcher = vi.fn<typeof fetch>()
  vi.stubGlobal('fetch', fetcher)
  render(<QueryClientProvider client={client}><App /></QueryClientProvider>)
  expect(await screen.findByText(message)).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Inspect existing data' })).not.toBeInTheDocument()
  if (!running) expect(screen.getByRole('button', { name: 'Retry sync' })).toBeEnabled()
  expect(fetcher).not.toHaveBeenCalled()
  client.clear()
})

it('allows explicit inspection after an ordinary sync failure', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  const bootstrap: Bootstrap = { defaults: { period: 'month', bucket: 'day', from: '', to: '', providers: [], models: [], harnesses: [], sessions: [] }, hostname: 'local', timezone: 'UTC' }
  const status: SyncStatus = { phase: 'failed', running: false, error: 'Sync failed.', revision: 1, harnesses: {} }
  client.setQueryData(['bootstrap'], bootstrap)
  client.setQueryData(['status'], status)
  render(<QueryClientProvider client={client}><App /></QueryClientProvider>)
  expect(await screen.findByRole('button', { name: 'Inspect existing data' })).toBeEnabled()
  client.clear()
})

it('cancels obsolete filter requests and only renders the current result', async () => {
  let aborted = false
  const fetcher = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>((input, init) => {
    if (String(input).includes('model=old')) return new Promise((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => { aborted = true; reject(new DOMException('Aborted', 'AbortError')) })
    })
    const data: Dashboard = { rows: [], chart: [], rowCount: 0, page: 1, pageSize: 50, range: 'week', lastSynced: 0, summary: { total: 123, input: 100, output: 23, cacheRead: 0, cacheWrite: 0, reasoning: 0, sessions: 1, syncedSessions: 10 } }
    return Promise.resolve(new Response(JSON.stringify(data), { headers: { 'Content-Type': 'application/json' } }))
  })
  vi.stubGlobal('fetch', fetcher)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const base = initialQuery({ period: 'week', bucket: 'day', from: '', to: '', providers: [], models: ['old'], harnesses: [], sessions: [] })
  function Example({ query }: { query: QueryState }) {
    const data = useAnalytics(query, 0, true)
    return <output>{data.data ? data.data.summary.total : 'Loading'}</output>
  }
  const view = render(<QueryClientProvider client={client}><Example query={base} /></QueryClientProvider>)
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1))
  view.rerender(<QueryClientProvider client={client}><Example query={{ ...base, models: ['new'] }} /></QueryClientProvider>)
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('123'))
  expect(aborted).toBe(true)
  client.clear()
})
