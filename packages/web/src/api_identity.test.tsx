import type { ReactNode } from 'react'
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, it, vi } from 'vitest'
import { useAnalytics, useFacets } from './api'
import { mockBootstrap, mockDashboard, mockFacets, mockSyncStatus } from './mocks/data'
import { initialQuery } from './state'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

for (const kind of ['usage', 'facets']) {
  for (const scenario of [
    'missing identity',
    'changed database',
    'stale revision',
    'zero revision',
    'newer revision',
  ]) {
    it(`${kind}: ${scenario} validates snapshot binding`, async () => {
      const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      const base = kind === 'usage' ? mockDashboard('tokens') : mockFacets
      const revision = scenario === 'zero revision' ? 0 : 1
      client.setQueryData(['sync'], mockSyncStatus(revision))
      const response =
        scenario === 'missing identity'
          ? Object.fromEntries(Object.entries(base).filter(([key]) => key !== 'dataEpoch'))
          : {
              ...base,
              dataEpoch: scenario === 'changed database' ? 'other-database' : base.dataEpoch,
              revision:
                scenario === 'stale revision' ? 0 : scenario === 'newer revision' ? 2 : revision,
            }
      vi.stubGlobal(
        'fetch',
        vi.fn<typeof fetch>(() => Promise.resolve(new Response(JSON.stringify(response)))),
      )
      function Wrapper({ children }: { children: ReactNode }) {
        return <QueryClientProvider client={client}>{children}</QueryClientProvider>
      }
      const query = initialQuery(mockBootstrap.defaults)
      const identity = `${mockBootstrap.instanceId}/${mockBootstrap.dataEpoch}`
      function useExample() {
        const analytics = useAnalytics(query, revision, kind === 'usage', identity)
        const facets = useFacets(query, revision, kind === 'facets', identity)
        return kind === 'usage' ? analytics : facets
      }
      const { result, unmount } = renderHook(useExample, { wrapper: Wrapper })
      const accepted = scenario === 'zero revision' || scenario === 'newer revision'
      await waitFor(() => expect(result.current.status).toBe(accepted ? 'success' : 'error'))
      expect(result.current.data === undefined).toBe(!accepted)
      const refreshStatus =
        scenario === 'changed database' ||
        scenario === 'stale revision' ||
        scenario === 'newer revision'
      expect(client.getQueryState(['sync'])?.isInvalidated).toBe(refreshStatus)
      unmount()
      client.clear()
    })
  }
}
