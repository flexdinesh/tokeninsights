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

it('guards repeated sync actions and allows recovery after a failed submission', async () => {
  let failing = true
  let posts = 0
  const ready: SyncStatus = {
    phase: 'ready',
    running: false,
    error: '',
    revision: 0,
    harnesses: {},
  }
  const running: SyncStatus = { ...ready, phase: 'syncing', running: true }
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>((_input, init) => {
      if (init?.method === 'POST') {
        posts += 1
        if (failing)
          return Promise.resolve(
            new Response(JSON.stringify({ code: 'unavailable', message: 'Sync unavailable' }), {
              status: 503,
            }),
          )
      }
      return Promise.resolve(new Response(JSON.stringify(failing ? ready : running)))
    }),
  )
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['sync'], ready)
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }
  const { result, unmount } = renderHook(useDashboardSync, { wrapper: Wrapper })
  act(() => {
    result.current.startSync()
    result.current.startSync()
  })
  await waitFor(() => expect(result.current.sync.error?.message).toBe('Sync unavailable'))
  await waitFor(() => expect(result.current.sync.isPending).toBe(false))
  expect(result.current.running).toBe(false)
  expect(posts).toBe(1)
  failing = false
  act(() => result.current.startSync())
  await waitFor(() => expect(result.current.statusQuery.data?.running).toBe(true))
  expect(result.current.running).toBe(true)
  expect(result.current.sync.error).toBeNull()
  expect(posts).toBe(2)
  unmount()
  client.clear()
})
