import { afterEach, expect, it, vi } from 'vitest'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { createAppRouter } from './router'
import type { Bootstrap, CollectorProgress, Dashboard, SyncStatus } from './contracts'
import { mockBootstrap, mockDashboard, mockFacets, mockSyncStatus } from './mocks/data'

const clients: QueryClient[] = []
function requestPath(input: RequestInfo | URL): string {
  return typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
}
afterEach(() => {
  cleanup()
  for (const client of clients.splice(0)) client.clear()
  vi.unstubAllGlobals()
  window.localStorage?.clear()
})

function progress(stage: CollectorProgress['attempts'][number]['stage']): CollectorProgress {
  return {
    instanceId: mockBootstrap.instanceId,
    attempts: [
      {
        attemptId: 'startup',
        stage,
        harnesses: { codex: 'running', pi: 'complete' },
        acknowledgedBatches: 0,
        acknowledgedEntries: 0,
        pendingKnown: false,
        pending: 0,
        startedAtMs: 1000,
        updatedAtMs: 1001,
        expiresAtMs: 16001,
      },
    ],
  }
}

function emptyDashboard(): Dashboard {
  return {
    ...mockDashboard('tokens'),
    rows: [],
    chart: [],
    rowCount: 0,
    summary: {
      total: 0,
      input: 0,
      output: 0,
      cacheRead: 0,
      cacheWrite: 0,
      reasoning: 0,
      sessions: 0,
      syncedSessions: 0,
    },
  }
}

function mountDashboard(
  options: {
    bootstrap?: Bootstrap
    status?: SyncStatus
    progress?: CollectorProgress
    dashboard?: Dashboard
    failure?: string
    response?: (path: string) => Promise<Response> | undefined
  } = {},
) {
  const state = {
    bootstrap: options.bootstrap ?? mockBootstrap,
    status: options.status ?? mockSyncStatus(1),
    progress: options.progress ?? { instanceId: mockBootstrap.instanceId, attempts: [] },
    dashboard: options.dashboard ?? mockDashboard('tokens'),
    failure: options.failure ?? '',
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  clients.push(client)
  client.setQueryData(['instance'], state.bootstrap)
  client.setQueryData(['sync', state.bootstrap.datasetId], state.status)
  const fetcher = vi.fn<typeof fetch>((input) => {
    const path = requestPath(input)
    const response = options.response?.(path)
    if (response) return response
    if (state.failure && path.includes(state.failure))
      return Promise.resolve(
        new Response(
          JSON.stringify({ code: 'unavailable', message: 'Request temporarily failed.' }),
          { status: 503 },
        ),
      )
    const body = path.endsWith('/instance')
      ? state.bootstrap
      : path.endsWith('/status')
        ? state.status
        : path.endsWith('/collector-progress')
          ? state.progress
          : path.includes('/usage/facets')
            ? { ...mockFacets, revision: state.status.revision }
            : state.dashboard
    return Promise.resolve(new Response(JSON.stringify(body)))
  })
  vi.stubGlobal('fetch', fetcher)
  render(
    <QueryClientProvider client={client}>
      <RouterProvider
        router={createAppRouter(createMemoryHistory({ initialEntries: ['/tokens'] }))}
      />
    </QueryClientProvider>,
  )
  return { client, fetcher, state }
}

it('local query Reload remains available without collection and disabled capability keeps machine context', async () => {
  const { client, state, fetcher } = mountDashboard()
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Reload' })).toBeEnabled()
  expect(
    fetcher.mock.calls.some(([input]) => requestPath(input).includes('collector-progress')),
  ).toBe(false)
  await act(() =>
    client.setQueryData(['instance'], {
      ...state.bootstrap,
      capabilities: state.bootstrap.capabilities.filter((value) => value !== 'dashboard-reload'),
    }),
  )
  expect(screen.queryByRole('button', { name: 'Reload' })).not.toBeInTheDocument()
  expect(screen.getByTitle(/Local machine running this dashboard/)).toHaveTextContent(
    mockBootstrap.hostname,
  )
})

it('hosted ignores inconsistent local capabilities and preserves saved usage without collector requests', async () => {
  const { fetcher } = mountDashboard({
    bootstrap: {
      ...mockBootstrap,
      serverKind: 'hosted',
      capabilities: [...mockBootstrap.capabilities, 'collector-progress'],
    },
  })
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Reload' })).not.toBeInTheDocument()
  expect(screen.queryByRole('region', { name: 'Collector progress' })).not.toBeInTheDocument()
  expect(
    fetcher.mock.calls.some(([input]) => requestPath(input).includes('collector-progress')),
  ).toBe(false)
})

it('a caller without read permission cannot expose Reload or collector progress', async () => {
  const { fetcher } = mountDashboard({
    bootstrap: {
      ...mockBootstrap,
      permissions: [],
      capabilities: [...mockBootstrap.capabilities, 'collector-progress'],
    },
  })
  expect(await screen.findByText('This server does not support the web dashboard.')).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Reload' })).not.toBeInTheDocument()
  expect(screen.queryByRole('region', { name: 'Collector progress' })).not.toBeInTheDocument()
  expect(fetcher).not.toHaveBeenCalled()
})

it('startup shares one progress request, names harness state and suppresses premature empty/Ready states', async () => {
  const { fetcher } = mountDashboard({
    bootstrap: {
      ...mockBootstrap,
      capabilities: [...mockBootstrap.capabilities, 'collector-progress'],
    },
    progress: progress('capturing'),
    dashboard: emptyDashboard(),
  })
  const region = await screen.findByRole('region', { name: 'Collector progress' })
  expect(region).toHaveTextContent('Collecting usage')
  expect(region).toHaveTextContent('codex: running')
  expect(region).toHaveTextContent('pi: complete')
  expect(within(screen.getByRole('banner')).getByText('Collecting usage')).toBeVisible()
  expect(screen.queryByText('Ready')).not.toBeInTheDocument()
  expect(screen.queryByText('No usage saved yet. Run tokeninsights sync.')).not.toBeInTheDocument()
  expect(screen.queryByText('No matching usage')).not.toBeInTheDocument()
  expect(
    fetcher.mock.calls.filter(([input]) => requestPath(input).includes('collector-progress')),
  ).toHaveLength(1)
})

it('acceptance refreshes status before declaring Ready and keeps processing visible until publication', async () => {
  let resolveStatus: ((response: Response) => void) | undefined
  const { client, state } = mountDashboard({
    bootstrap: {
      ...mockBootstrap,
      capabilities: [...mockBootstrap.capabilities, 'collector-progress'],
    },
    progress: progress('capturing'),
    dashboard: emptyDashboard(),
    response: (path) =>
      path.endsWith('/status')
        ? new Promise((resolve) => {
            resolveStatus = resolve
          })
        : undefined,
  })
  await screen.findByRole('region', { name: 'Collector progress' })
  await act(() =>
    client.setQueryData(['collector-progress', mockBootstrap.instanceId], progress('accepted')),
  )
  expect(await within(screen.getByRole('banner')).findByText('Loading…')).toBeVisible()
  expect(screen.queryByText('No usage saved yet. Run tokeninsights sync.')).not.toBeInTheDocument()
  await waitFor(() => expect(resolveStatus).toBeDefined())
  state.status = { ...state.status, pending: 1 }
  await act(async () => {
    resolveStatus?.(new Response(JSON.stringify(state.status)))
  })
  expect(await within(screen.getByRole('banner')).findByText('Processing usage')).toBeVisible()
  expect(screen.queryByText('No usage saved yet. Run tokeninsights sync.')).not.toBeInTheDocument()
  state.dashboard = { ...mockDashboard('tokens'), revision: 2 }
  state.status = { ...state.status, pending: 0, revision: 2 }
  await act(() => client.setQueryData(['sync', mockBootstrap.datasetId], state.status))
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  expect(await within(screen.getByRole('banner')).findByText('Ready')).toBeVisible()
})

it('rebuild and processing backoff retain saved totals; due retries return to processing', async () => {
  const { client, state } = mountDashboard({
    status: {
      ...mockSyncStatus(1),
      targetGeneration: 2,
      pending: 3,
      failed: 1,
      failedRetryAtMs: Date.now() + 60_000,
    },
  })
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  expect(
    screen.getByText('Rebuilding saved usage. Existing totals remain available.'),
  ).toBeVisible()
  expect(within(screen.getByRole('banner')).getByText('Processing retry pending')).toBeVisible()
  expect(screen.getByText(/Retry scheduled at/)).toBeVisible()
  await act(() =>
    client.setQueryData(['sync', mockBootstrap.datasetId], { ...state.status, failedRetryAtMs: 1 }),
  )
  expect(await within(screen.getByRole('banner')).findByText('Processing usage')).toBeVisible()
  expect(screen.getByText(/Retrying usage processing/)).toBeVisible()
  expect(screen.getByLabelText('Total tokens: 647,000')).toBeVisible()
})

it('filter failure reports its own status and retries without hiding saved usage or machine identity', async () => {
  const { state } = mountDashboard({ failure: '/usage/facets' })
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  expect(await within(screen.getByRole('banner')).findByText('Filters couldn’t load')).toBeVisible()
  expect(screen.queryByText('Connection unavailable')).not.toBeInTheDocument()
  expect(screen.getByTitle(/Local machine running this dashboard/)).toHaveTextContent(
    mockBootstrap.hostname,
  )
  state.failure = ''
  await userEvent.click(
    within(screen.getByRole('alert')).getByRole('button', { name: 'Retry Request' }),
  )
  expect(await within(screen.getByRole('banner')).findByText('Ready')).toBeVisible()
})

it('missing collection status shows recovery without Ready, premature empty text or endless loading', async () => {
  mountDashboard({
    bootstrap: {
      ...mockBootstrap,
      capabilities: [...mockBootstrap.capabilities, 'collector-progress'],
    },
    failure: '/collector-progress',
    dashboard: emptyDashboard(),
  })
  expect(
    await within(await screen.findByRole('banner')).findByText('Collection status unavailable'),
  ).toBeVisible()
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Collection status couldn’t load. Saved usage remains available.',
  )
  expect(screen.queryByText('Ready')).not.toBeInTheDocument()
  expect(screen.queryByText('No usage saved yet. Run tokeninsights sync.')).not.toBeInTheDocument()
  await waitFor(() =>
    expect(screen.queryByRole('status', { name: 'Loading dashboard' })).not.toBeInTheDocument(),
  )
})

it('caught-up processing does not declare Ready while the final usage query is still loading', async () => {
  let resolveUsage: ((response: Response) => void) | undefined
  mountDashboard({
    response: (path) =>
      path.includes('/usage?')
        ? new Promise((resolve) => {
            resolveUsage = resolve
          })
        : undefined,
  })
  const header = await screen.findByRole('banner')
  expect(await within(header).findByText('Loading…')).toBeVisible()
  expect(screen.getByRole('status', { name: 'Loading dashboard' })).toBeVisible()
  expect(within(header).queryByText('Ready')).not.toBeInTheDocument()
  await waitFor(() => expect(resolveUsage).toBeDefined())
  await act(async () => {
    resolveUsage?.(new Response(JSON.stringify(mockDashboard('tokens'))))
  })
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  expect(await within(header).findByText('Ready')).toBeVisible()
})

it('collection failure shows saved data and names the recovery', async () => {
  const { client, state } = mountDashboard({
    bootstrap: {
      ...mockBootstrap,
      capabilities: [...mockBootstrap.capabilities, 'collector-progress'],
    },
    progress: progress('failed'),
  })
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  expect(
    await within(screen.getByRole('banner')).findByText('Collection needs attention'),
  ).toBeVisible()
  expect(screen.getByRole('alert')).toHaveTextContent('Run tokeninsights sync to retry collection.')
  await act(() =>
    client.setQueryData(['instance'], {
      ...state.bootstrap,
      capabilities: state.bootstrap.capabilities.filter(
        (capability) => capability !== 'collector-progress',
      ),
    }),
  )
  expect(await within(screen.getByRole('banner')).findByText('Ready')).toBeVisible()
  expect(screen.queryByRole('region', { name: 'Collector progress' })).not.toBeInTheDocument()
})

it('retained completion history stays compact while every running attempt remains visible', async () => {
  const complete = progress('accepted').attempts[0]
  const running = progress('capturing').attempts[0]
  if (!complete || !running) throw new Error('Missing progress fixture')
  mountDashboard({
    bootstrap: {
      ...mockBootstrap,
      capabilities: [...mockBootstrap.capabilities, 'collector-progress'],
    },
    progress: {
      instanceId: mockBootstrap.instanceId,
      attempts: [
        ...Array.from({ length: 20 }, (_, index) => ({
          ...complete,
          attemptId: `complete-${index}`,
          updatedAtMs: 1000 + index,
        })),
        { ...running, attemptId: 'running-one', updatedAtMs: 1020 },
        { ...running, attemptId: 'running-two', updatedAtMs: 1021 },
      ],
    },
  })
  const region = await screen.findByRole('region', { name: 'Collector progress' })
  expect(within(region).getAllByText(/Usage accepted/)).toHaveLength(1)
  expect(within(region).getAllByText(/Collecting usage/)).toHaveLength(2)
  expect(within(region).getAllByRole('list', { name: 'Harness collection' })).toHaveLength(3)
  expect(within(region).getByText('Harness collection').closest('details')).not.toHaveAttribute(
    'open',
  )
})
