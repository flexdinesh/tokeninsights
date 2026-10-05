import type { ReactNode } from 'react'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, it, vi } from 'vitest'
import type { SyncStatus } from './contracts'
import { useDashboardSync } from './useDashboardSync'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

const ready: SyncStatus = {
  phase: 'ready',
  running: false,
  error: '',
  revision: 7,
  harnesses: {},
  instanceId: 'instance',
  dataEpoch: 'old',
  dataReadiness: 'ready',
}

function setup(initial: SyncStatus | null = ready) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  if (initial) client.setQueryData(['sync'], initial)
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }
  return { client, ...renderHook(useDashboardSync, { wrapper: Wrapper }) }
}

it('Reload reads status and invalidates saved queries without submitting collection', async () => {
  const fetcher = vi.fn<typeof fetch>(() => Promise.resolve(new Response(JSON.stringify(ready))))
  vi.stubGlobal('fetch', fetcher)
  const { result, client, unmount } = setup()
  client.setQueryData(['usage', 'saved'], { total: 10 })
  client.setQueryData(['facets', 'saved'], { models: ['model'] })
  expect(result.current.analyticsEnabled).toBe(true)
  expect(fetcher).not.toHaveBeenCalled()
  await act(() => result.current.reload())
  expect(fetcher).toHaveBeenCalled()
  expect(fetcher.mock.calls.every(([, init]) => init?.method === 'GET')).toBe(true)
  expect(client.getQueryState(['usage', 'saved'])?.isInvalidated).toBe(true)
  expect(client.getQueryState(['facets', 'saved'])?.isInvalidated).toBe(true)
  unmount()
  client.clear()
})

it('database identity changes remove old analytics and facets', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(() => Promise.resolve(new Response(JSON.stringify(ready)))),
  )
  const { result, client, unmount } = setup()
  client.setQueryData(['usage', 'old'], { saved: true })
  client.setQueryData(['facets', 'old'], { saved: true })
  await act(() => client.setQueryData(['sync'], { ...ready, dataEpoch: 'new' }))
  await waitFor(() => expect(result.current.identity).toBe('instance/new'))
  expect(client.getQueryData(['usage', 'old'])).toBeUndefined()
  expect(client.getQueryData(['facets', 'old'])).toBeUndefined()
  unmount()
  client.clear()
})

it('a committed ingestion revision invalidates saved queries and producer labels', async () => {
  const { result, client, unmount } = setup()
  for (const key of ['usage', 'facets', 'instance']) {
    client.setQueryData([key, 'saved'], { saved: true })
  }
  await act(() => client.setQueryData(['sync'], { ...ready, revision: 8 }))
  await waitFor(() => expect(result.current.revision).toBe(8))
  for (const key of ['usage', 'facets', 'instance']) {
    expect(client.getQueryState([key, 'saved'])?.isInvalidated).toBe(true)
  }
  unmount()
  client.clear()
})

for (const readiness of ['metadata', 'recovery', 'rebuild', 'unavailable']) {
  it(`${readiness} status with an explicitly absent database disables analytics and clears replaced data`, async () => {
    const fetcher = vi.fn<typeof fetch>(() =>
      Promise.resolve(
        new Response(JSON.stringify({ ...ready, dataReadiness: readiness, dataEpoch: '' })),
      ),
    )
    vi.stubGlobal('fetch', fetcher)
    const { result, client, unmount } = setup()
    client.setQueryData(['usage', 'old'], { saved: true })
    client.setQueryData(['facets', 'old'], { saved: true })
    await act(() => result.current.reload())
    await waitFor(() => expect(result.current.analyticsEnabled).toBe(false))
    expect(result.current.identity).toBe('instance/')
    expect(client.getQueryData(['usage', 'old'])).toBeUndefined()
    expect(client.getQueryData(['facets', 'old'])).toBeUndefined()
    expect(fetcher.mock.calls.every(([, init]) => init?.method === 'GET')).toBe(true)
    unmount()
    client.clear()
  })
}

it('missing readiness in a status response cannot enable analytics', async () => {
  const malformed = Object.fromEntries(
    Object.entries(ready).filter(([key]) => key !== 'dataReadiness'),
  )
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(() => Promise.resolve(new Response(JSON.stringify(malformed)))),
  )
  const { result, client, unmount } = setup(null)
  await waitFor(() => expect(result.current.statusQuery.isError).toBe(true))
  expect(result.current.analyticsEnabled).toBe(false)
  expect(result.current.identity).toBe('')
  unmount()
  client.clear()
})
