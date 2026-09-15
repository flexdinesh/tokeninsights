import { delay, http, HttpResponse } from 'msw'
import { mockBootstrap, mockDashboard, mockFacets, mockSyncStatus } from './data'

let revision = 1
let syncDeadline = 0
let completionPending = false

function currentSyncStatus() {
  const running = Date.now() < syncDeadline
  if (!running && completionPending) {
    revision += 1
    completionPending = false
  }
  return mockSyncStatus(running, revision)
}

export const handlers = [
  http.get('*/api/v1/instance', async () => {
    await delay(100)
    return HttpResponse.json(mockBootstrap)
  }),
  http.get('*/api/v1/sync', async () => {
    await delay(100)
    return HttpResponse.json(currentSyncStatus())
  }),
  http.post('*/api/v1/sync', async () => {
    syncDeadline = Date.now() + 1_500
    completionPending = true
    await delay(100)
    return HttpResponse.json(currentSyncStatus(), { status: 202 })
  }),
  http.get('*/api/v1/usage/facets', async () => {
    await delay(100)
    return HttpResponse.json(mockFacets)
  }),
  http.get('*/api/v1/usage', async ({ request }) => {
    const params = new URL(request.url).searchParams
    await delay(150)
    return HttpResponse.json(
      mockDashboard(params.get('tab'), params.get('page'), params.get('pageSize')),
    )
  }),
]
