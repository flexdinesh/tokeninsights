import { afterEach, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { createAppRouter } from './router'
import { mockBootstrap, mockDashboard, mockFacets, mockSyncStatus } from './mocks/data'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

function requestPath(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  return input instanceof URL ? input.href : input.url
}

function renderApp() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createAppRouter(createMemoryHistory({ initialEntries: ['/tokens'] }))
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return client
}

it('hosted token login opens usage without ever requesting collector progress', async () => {
  let authenticated = false
  const fetcher = vi.fn<typeof fetch>((input) => {
    const path = requestPath(input)
    if (path.endsWith('/auth/session')) {
      authenticated = true
      return Promise.resolve(new Response(null, { status: 204 }))
    }
    if (!authenticated) return Promise.resolve(new Response(null, { status: 401 }))
    const body = path.endsWith('/instance')
      ? {
          ...mockBootstrap,
          serverKind: 'hosted',
          capabilities: ['usage', 'facets', 'web-dashboard', 'collector-progress'],
        }
      : path.endsWith('/status')
        ? mockSyncStatus(1)
        : path.includes('/facets')
          ? mockFacets
          : mockDashboard('tokens')
    return Promise.resolve(new Response(JSON.stringify(body)))
  })
  vi.stubGlobal('fetch', fetcher)
  const client = renderApp()
  fireEvent.change(await screen.findByLabelText('Access token'), {
    target: { value: 'account-token' },
  })
  await userEvent.click(screen.getByRole('button', { name: 'Sign in' }))
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v2/auth/session',
    expect.objectContaining({ method: 'POST', body: JSON.stringify({ token: 'account-token' }) }),
  )
  expect(screen.queryByLabelText('Collector progress')).not.toBeInTheDocument()
  expect(
    fetcher.mock.calls.some(([input]) => requestPath(input).includes('collector-progress')),
  ).toBe(false)
  client.clear()
})

it('personal progress is fetched only when the server advertises it', async () => {
  const fetcher = vi.fn<typeof fetch>((input) => {
    const path = requestPath(input)
    const body = path.endsWith('/instance')
      ? { ...mockBootstrap, capabilities: [...mockBootstrap.capabilities, 'collector-progress'] }
      : path.endsWith('/collector-progress')
        ? {
            instanceId: mockBootstrap.instanceId,
            attempts: [
              {
                attemptId: 'attempt',
                stage: 'submitting',
                harnesses: { codex: 'running' },
                acknowledgedBatches: 1,
                acknowledgedEntries: 12,
                pendingKnown: false,
                pending: 0,
                startedAtMs: 1000,
                updatedAtMs: 2000,
                expiresAtMs: 17000,
              },
            ],
          }
        : path.endsWith('/status')
          ? mockSyncStatus(1)
          : path.includes('/facets')
            ? mockFacets
            : mockDashboard('tokens')
    return Promise.resolve(new Response(JSON.stringify(body)))
  })
  vi.stubGlobal('fetch', fetcher)
  const client = renderApp()
  expect(await screen.findByLabelText('Collector progress')).toHaveTextContent('Submitting usage')
  await waitFor(() =>
    expect(
      fetcher.mock.calls.some(([input]) => requestPath(input).endsWith('/collector-progress')),
    ).toBe(true),
  )
  client.clear()
})

it('revoked browser sessions hide and clear cached usage immediately', async () => {
  let revoked = false
  const fetcher = vi.fn<typeof fetch>((input) => {
    const path = requestPath(input)
    if (revoked) return Promise.resolve(new Response(null, { status: 401 }))
    const body = path.endsWith('/instance')
      ? { ...mockBootstrap, serverKind: 'hosted' }
      : path.endsWith('/status')
        ? mockSyncStatus(1)
        : path.includes('/facets')
          ? mockFacets
          : mockDashboard('tokens')
    return Promise.resolve(new Response(JSON.stringify(body)))
  })
  vi.stubGlobal('fetch', fetcher)
  const client = renderApp()
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  revoked = true
  await userEvent.click(screen.getByRole('button', { name: 'Reload' }))
  expect(await screen.findByRole('heading', { name: 'Sign in to TokenInsights' })).toBeVisible()
  expect(screen.queryByLabelText('Total tokens: 647,000')).not.toBeInTheDocument()
  await waitFor(() =>
    expect(
      client
        .getQueryCache()
        .getAll()
        .every((query) => query.state.data === undefined),
    ).toBe(true),
  )
  client.clear()
})
