import { delay, http, HttpResponse } from 'msw'
import { mockBootstrap, mockDashboard, mockFacets, mockSyncStatus } from './data'

const revision = 1

export const handlers = [
  http.get('*/api/v2/instance', async () => {
    await delay(100)
    return HttpResponse.json(mockBootstrap)
  }),
  http.get('*/api/v2/status', async () => {
    await delay(100)
    return HttpResponse.json(mockSyncStatus(revision))
  }),
  http.get('*/api/v2/usage/facets', async () => {
    await delay(100)
    return HttpResponse.json({ ...mockFacets, revision })
  }),
  http.get('*/api/v2/usage', async ({ request }) => {
    const params = new URL(request.url).searchParams
    await delay(150)
    return HttpResponse.json({
      ...mockDashboard(params.get('tab'), params.get('page'), params.get('pageSize')),
      revision,
    })
  }),
]
