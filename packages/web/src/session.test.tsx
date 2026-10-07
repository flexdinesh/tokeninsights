import type { ReactNode } from 'react'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, it, vi } from 'vitest'
import { changeBrowserSession, useAnalytics } from './api'
import { clearAccountQueries } from './session'
import { mockBootstrap, mockDashboard } from './mocks/data'
import { initialQuery } from './state'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

it('changing accounts cancels old reads and clears all cached account data', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  let aborted = false
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(
      (_path, init) =>
        new Promise((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => {
            aborted = true
            reject(new DOMException('Aborted', 'AbortError'))
          })
        }),
    ),
  )
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }
  client.setQueryData(['facets', 'old'], { sessions: ['private-session'] })
  client.setQueryData(['collector-progress'], { attempts: ['private-attempt'] })
  const view = renderHook(
    () => useAnalytics(initialQuery(mockBootstrap.defaults), 1, true, 'instance/database/user-a/1'),
    { wrapper: Wrapper },
  )
  await waitFor(() => expect(view.result.current.isFetching).toBe(true))
  await act(() => clearAccountQueries(client))
  expect(aborted).toBe(true)
  expect(client.getQueryCache().getAll()).toHaveLength(0)
  view.unmount()
})

it('never uses another dataset as placeholder data while switching accounts', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  let first = true
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(() => {
      if (first) {
        first = false
        return Promise.resolve(new Response(JSON.stringify(mockDashboard('tokens'))))
      }
      return new Promise<Response>(() => undefined)
    }),
  )
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }
  const view = renderHook(
    ({ identity }) => useAnalytics(initialQuery(mockBootstrap.defaults), 1, true, identity),
    { wrapper: Wrapper, initialProps: { identity: 'mock-instance/mock-epoch/default/1' } },
  )
  await waitFor(() => expect(view.result.current.isSuccess).toBe(true))
  view.rerender({ identity: 'mock-instance/mock-epoch/user-b/1' })
  expect(view.result.current.data).toBeUndefined()
  expect(view.result.current.isPlaceholderData).toBe(false)
  view.unmount()
  client.clear()
})

it('exchanges tokens in request body without persisting them', async () => {
  const fetcher = vi.fn<typeof fetch>(() => Promise.resolve(new Response(null, { status: 204 })))
  vi.stubGlobal('fetch', fetcher)
  await changeBrowserSession('secret-token')
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v2/auth/session',
    expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ token: 'secret-token' }),
      credentials: 'same-origin',
    }),
  )
  await changeBrowserSession()
  expect(fetcher).toHaveBeenLastCalledWith(
    '/api/v2/auth/session',
    expect.objectContaining({ method: 'DELETE', body: undefined }),
  )
})
