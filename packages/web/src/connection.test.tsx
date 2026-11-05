import type { ReactNode } from 'react'
import { act, cleanup, render, renderHook, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { afterEach, expect, it, vi } from 'vitest'
import { viewerConnectionSchema } from '@tokeninsights/contracts'
import type { DashboardClient, ViewerConnection } from '@tokeninsights/contracts'
import { DashboardClientProvider, createAppRouter } from '@tokeninsights/dashboard'
import { useAnalytics } from '../../dashboard/src/api'
import { initialQuery, parseDashboardSearch } from '../../dashboard/src/query'
import {
  mockBootstrap,
  mockConnection,
  mockDashboard,
  mockFacets,
  mockSyncStatus,
} from './mocks/data'
import { httpDashboardClient } from './http-client'

const clients: QueryClient[] = []
afterEach(() => {
  cleanup()
  for (const client of clients.splice(0)) client.clear()
  vi.unstubAllGlobals()
})

function mount(
  options: { connection?: ViewerConnection; entry?: string; bootstrapFailure?: Error } = {},
) {
  const api = {
    getInstance: vi.fn<DashboardClient['getInstance']>().mockImplementation(async () => {
      if (options.bootstrapFailure) throw options.bootstrapFailure
      return mockBootstrap
    }),
    getConnection: vi
      .fn<DashboardClient['getConnection']>()
      .mockResolvedValue(options.connection ?? mockConnection),
    getStatus: vi.fn<DashboardClient['getStatus']>().mockResolvedValue(mockSyncStatus(1)),
    getUsage: vi.fn<DashboardClient['getUsage']>().mockImplementation(async (query) => ({
      ...mockDashboard(query.tab),
      quality: query.quality,
    })),
    getFacets: vi.fn<DashboardClient['getFacets']>().mockResolvedValue(mockFacets),
    getCollectorProgress: vi.fn<DashboardClient['getCollectorProgress']>().mockResolvedValue({
      instanceId: mockBootstrap.instanceId,
      attempts: [],
    }),
    refresh: vi.fn<DashboardClient['refresh']>().mockResolvedValue(),
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  clients.push(client)
  const router = createAppRouter(
    createMemoryHistory({ initialEntries: [options.entry ?? '/tokens'] }),
  )
  const view = render(
    <DashboardClientProvider client={api}>
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </DashboardClientProvider>,
  )
  return { api, client, router, view }
}

it('Refresh invokes capture once, blocks repeat clicks, and preserves saved usage during failure', async () => {
  const { api } = mount()
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  let rejectRefresh: ((error: Error) => void) | undefined
  api.refresh.mockImplementation(
    () =>
      new Promise((_resolve, reject) => {
        rejectRefresh = reject
      }),
  )
  const refresh = screen.getByRole('button', { name: 'Refresh' })
  await userEvent.click(refresh)
  expect(screen.getByRole('button', { name: 'Refreshing…' })).toBeDisabled()
  await userEvent.click(refresh)
  expect(api.refresh).toHaveBeenCalledTimes(1)
  expect(screen.getByLabelText('Total tokens: 647,000')).toBeVisible()
  await act(async () => rejectRefresh?.(new Error('Connection closed')))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Refresh failed. Saved usage remains available.',
  )
  expect(screen.getByLabelText('Total tokens: 647,000')).toBeVisible()
  expect(await screen.findByRole('button', { name: 'Refresh' })).toBeEnabled()
  expect(api.refresh).toHaveBeenCalledTimes(1)
})

it('filtering and query reload never request capture', async () => {
  const { api, client, router } = mount()
  await screen.findByLabelText('Total tokens: 647,000')
  await act(() =>
    router.navigate({
      to: '/$tab',
      params: { tab: 'models' },
      search: parseDashboardSearch({ models: ['gpt-5'] }),
    }),
  )
  await waitFor(() =>
    expect(api.getUsage).toHaveBeenCalledWith(
      expect.objectContaining({ tab: 'models', models: ['gpt-5'] }),
      expect.any(AbortSignal),
    ),
  )
  await act(() => client.invalidateQueries({ queryKey: ['usage'] }))
  expect(api.refresh).not.toHaveBeenCalled()
})

for (const transport of ['ssh', 'tcp'] satisfies ViewerConnection['transport'][]) {
  it(`${transport} target and verified backend remain visible when the connection fails`, async () => {
    const { api, client } = mount({
      connection: { transport, target: 'remote.example:443', state: 'connected' },
    })
    await screen.findByLabelText('Total tokens: 647,000')
    expect(await screen.findByLabelText('Connection state')).toHaveTextContent(
      `${transport.toUpperCase()} · remote.example:443 · connected`,
    )
    expect(screen.getByTitle(/Backend hostname:/)).toHaveAttribute(
      'title',
      `Backend hostname: ${mockBootstrap.hostname} · Identity: ${mockBootstrap.instanceId}`,
    )
    const bootstrapReads = api.getInstance.mock.calls.length
    api.getStatus.mockRejectedValue(new Error('Connection closed'))
    await act(() => client.invalidateQueries({ queryKey: ['sync'] }))
    expect(await screen.findByLabelText('Connection state')).toHaveTextContent(
      `${transport.toUpperCase()} · remote.example:443 · unavailable`,
    )
    expect(screen.getByLabelText('Total tokens: 647,000')).toBeVisible()
    expect(within(screen.getByRole('banner')).queryByText(/Local/)).not.toBeInTheDocument()
    expect(api.getInstance).toHaveBeenCalledTimes(bootstrapReads)
  })
}

for (const route of ['/settings', '/login', '/signup', '/account']) {
  it(`retired account route ${route} has no screen`, async () => {
    const { api } = mount({ entry: route })
    expect(await screen.findByText('Dashboard route not found.')).toBeVisible()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(api.getUsage).not.toHaveBeenCalled()
    expect(api.refresh).not.toHaveBeenCalled()
  })
}

it('initial transport failure still shows the requested remote target without local data', async () => {
  const { api } = mount({
    connection: { transport: 'tcp', target: 'tcp://remote.example:443', state: 'connected' },
    bootstrapFailure: new Error('Backend unavailable'),
  })
  expect(
    await screen.findByRole('heading', { name: 'Couldn’t connect to tcp://remote.example:443' }),
  ).toBeVisible()
  expect(screen.getByRole('status')).toHaveTextContent(
    'TCP · tcp://remote.example:443 · unavailable',
  )
  expect(screen.queryByText('Local workspace')).not.toBeInTheDocument()
  expect(api.getUsage).not.toHaveBeenCalled()
  expect(api.refresh).not.toHaveBeenCalled()
})

it('connection metadata accepts only redacted display fields', async () => {
  const connection = {
    transport: 'tcp',
    target: 'remote.example:443',
    state: 'connected',
    hostname: 'remote',
  }
  const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify(connection)))
  vi.stubGlobal('fetch', fetcher)
  expect(await httpDashboardClient.getConnection()).toEqual(connection)
  expect(fetcher).toHaveBeenCalledExactlyOnceWith(
    '/api/viewer/connection',
    expect.objectContaining({ method: 'GET' }),
  )
  for (const secret of [
    { token: 'secret' },
    { pin: 'private' },
    { connectionString: 'credential-uri' },
  ]) {
    expect(viewerConnectionSchema.safeParse({ ...connection, ...secret }).success).toBe(false)
  }
})

it('failed publication query retains saved data only for its backend and selection', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  clients.push(client)
  const api = {
    ...httpDashboardClient,
    getUsage: vi
      .fn<DashboardClient['getUsage']>()
      .mockResolvedValueOnce(mockDashboard('tokens'))
      .mockRejectedValue(new Error('Offline')),
  }
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <DashboardClientProvider client={api}>
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      </DashboardClientProvider>
    )
  }
  const query = initialQuery(mockBootstrap.defaults)
  const view = renderHook(
    ({ revision, identity, models }) =>
      useAnalytics({ ...query, models }, revision, true, identity),
    {
      wrapper: Wrapper,
      initialProps: {
        revision: 1,
        identity: 'mock-instance/mock-epoch/default/1',
        models: new Array<string>(),
      },
    },
  )
  await waitFor(() => expect(view.result.current.isSuccess).toBe(true))
  view.rerender({ revision: 2, identity: 'mock-instance/mock-epoch/default/1', models: [] })
  await waitFor(() => expect(view.result.current.isError).toBe(true))
  expect(view.result.current.data?.dashboard.summary.total).toBe(647_000)
  view.rerender({ revision: 2, identity: 'other-instance/other-epoch/default/1', models: [] })
  expect(view.result.current.data).toBeUndefined()
  view.rerender({
    revision: 2,
    identity: 'mock-instance/mock-epoch/default/1',
    models: ['other-model'],
  })
  expect(view.result.current.data).toBeUndefined()
})
