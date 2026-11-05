import { DashboardClientProvider } from '@tokeninsights/dashboard'
import { httpDashboardClient } from './http-client'
import type { ReactNode } from 'react'
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, it, vi } from 'vitest'
import { useAnalytics } from '../../dashboard/src/api'
import { mockBootstrap, mockDashboard } from './mocks/data'
import { initialQuery } from '../../dashboard/src/query'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

it('never uses another backend identity as placeholder data', async () => {
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
    return (
      <DashboardClientProvider client={httpDashboardClient}>
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      </DashboardClientProvider>
    )
  }
  const view = renderHook(
    ({ identity }) => useAnalytics(initialQuery(mockBootstrap.defaults), 1, true, identity),
    { wrapper: Wrapper, initialProps: { identity: 'mock-instance/mock-epoch/default/1' } },
  )
  await waitFor(() => expect(view.result.current.isSuccess).toBe(true))
  view.rerender({ identity: 'other-instance/other-epoch/default/1' })
  expect(view.result.current.data).toBeUndefined()
  expect(view.result.current.isPlaceholderData).toBe(false)
  view.unmount()
  client.clear()
})
