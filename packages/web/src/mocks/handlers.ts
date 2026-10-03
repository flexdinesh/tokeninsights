import { delay, http, HttpResponse } from 'msw'
import { mockBootstrap, mockDashboard, mockFacets, mockSyncStatus } from './data'

let revision = 1
let syncDeadline = 0
let completionPending = false
let pendingRefresh = false
let requestedAt = 0

function currentSyncStatus() {
  let running = Date.now() < syncDeadline
  if (!running && completionPending) {
    revision += 1
    completionPending = false
    if (pendingRefresh) {
      pendingRefresh = false
      syncDeadline = Date.now() + 1500
      completionPending = true
      running = true
    } else {
      requestedAt = 0
    }
  }
  return { ...mockSyncStatus(running, revision), pendingRefresh, checkRequestedAt: requestedAt }
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
    if (Date.now() < syncDeadline) {
      pendingRefresh = true
    } else {
      syncDeadline = Date.now() + 1_500
    }
    requestedAt = Date.now()
    completionPending = true
    await delay(100)
    return HttpResponse.json(currentSyncStatus(), { status: 202 })
  }),
  http.get('*/api/v1/usage/facets', async () => {
    await delay(100)
    return HttpResponse.json({ ...mockFacets, revision })
  }),
  http.get('*/api/v1/usage', async ({ request }) => {
    const params = new URL(request.url).searchParams
    await delay(150)
    return HttpResponse.json({
      ...mockDashboard(params.get('tab'), params.get('page'), params.get('pageSize')),
      revision,
    })
  }),
]
