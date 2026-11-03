import { act, cleanup, render, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import type { Bootstrap, DashboardClient } from '@tokeninsights/contracts'
import { DashboardClientProvider } from '@tokeninsights/dashboard'
import { useAnalytics, useBootstrap } from '@tokeninsights/dashboard/api'
import { initialQuery } from '../../dashboard/src/query'
import { mockBootstrap, mockDashboard } from './mocks/data'
import { httpDashboardClient } from './http-client'
import { BrowserViewer } from './viewer'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

it('browser lifetime observes bootstrap without issuing requests or bypassing authentication', () => {
  const fetcher = vi.fn<typeof fetch>().mockRejectedValue(new Error('Unexpected request'))
  vi.stubGlobal('fetch', fetcher)
  const close = vi.fn<() => void>()
  class ViewerStream {
    url: string
    constructor(url: string) {
      this.url = url
    }
    close = close
  }
  const eventSource = vi.fn<typeof ViewerStream>(ViewerStream)
  vi.stubGlobal('EventSource', eventSource)
  const client = new QueryClient()
  const view = render(
    <QueryClientProvider client={client}>
      <BrowserViewer />
    </QueryClientProvider>,
  )
  expect(eventSource).not.toHaveBeenCalled()
  act(() => {
    client.setQueryData(['instance'], mockBootstrap)
  })
  expect(eventSource).toHaveBeenCalledTimes(1)
  act(() => {
    client.setQueryData(['instance'], { ...mockBootstrap, hostname: 'same-instance' })
  })
  expect(eventSource).toHaveBeenCalledTimes(1)
  act(() => client.clear())
  expect(close).toHaveBeenCalledTimes(1)
  expect(fetcher).not.toHaveBeenCalled()
  view.unmount()
})

it('dashboard hooks use injected semantic operations without an HTTP transport', async () => {
  const fetcher = vi.fn<typeof fetch>().mockRejectedValue(new Error('Unexpected HTTP request'))
  vi.stubGlobal('fetch', fetcher)
  const getInstance = vi.fn<DashboardClient['getInstance']>().mockResolvedValue(mockBootstrap)
  const getUsage = vi.fn<DashboardClient['getUsage']>().mockResolvedValue(mockDashboard('tokens'))
  const api = { ...httpDashboardClient, getInstance, getUsage }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <DashboardClientProvider client={api}>
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      </DashboardClientProvider>
    )
  }
  const query = initialQuery(mockBootstrap.defaults)
  const view = renderHook(
    () => ({
      instance: useBootstrap(),
      usage: useAnalytics(query, 1, true, 'mock-instance/mock-epoch/default/1'),
    }),
    { wrapper: Wrapper },
  )
  await waitFor(() => expect(view.result.current.usage.isSuccess).toBe(true))
  expect(view.result.current.instance.data).toEqual(mockBootstrap)
  expect(getUsage).toHaveBeenCalledWith(query, expect.any(AbortSignal))
  expect(fetcher).not.toHaveBeenCalled()
  view.unmount()
  client.clear()
})

for (const kind of ['personal', 'hosted'] satisfies Bootstrap['serverKind'][]) {
  it(`${kind} browser viewer owns only a local instance stream`, async () => {
    const close = vi.fn<() => void>()
    class ViewerStream {
      url: string
      constructor(url: string) {
        this.url = url
      }
      close = close
    }
    const eventSource = vi.fn<typeof ViewerStream>(ViewerStream)
    vi.stubGlobal('EventSource', eventSource)
    const api = {
      ...httpDashboardClient,
      getInstance: vi
        .fn<DashboardClient['getInstance']>()
        .mockResolvedValue({ ...mockBootstrap, serverKind: kind }),
    }
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    function Wrapper({ children }: { children: ReactNode }) {
      return (
        <DashboardClientProvider client={api}>
          <QueryClientProvider client={client}>{children}</QueryClientProvider>
        </DashboardClientProvider>
      )
    }
    const view = renderHook(
      () => {
        BrowserViewer()
        return useBootstrap()
      },
      { wrapper: Wrapper },
    )
    await waitFor(() => expect(view.result.current.isSuccess).toBe(true))
    expect(eventSource.mock.calls).toEqual(kind === 'personal' ? [['/api/local/v1/viewers']] : [])
    view.unmount()
    expect(close).toHaveBeenCalledTimes(kind === 'personal' ? 1 : 0)
    client.clear()
  })
}
