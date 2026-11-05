import {
  bootstrapSchema,
  collectorProgressSchema,
  dashboardSchema,
  facetsSchema,
  statusSchema,
  viewerConnectionSchema,
} from '@tokeninsights/contracts'
import type { DashboardClient } from '@tokeninsights/contracts'
import { request, send } from './http'
import { apiQueryParams } from './http-query'

// The native bridge owns connection transport and credentials.
export const httpDashboardClient: DashboardClient = {
  getInstance: (signal) => request('/api/v2/instance', bootstrapSchema, signal),
  getStatus: (signal) => request('/api/v2/status', statusSchema, signal),
  getUsage: (query, signal) =>
    request(`/api/v2/usage?${apiQueryParams(query)}`, dashboardSchema, signal),
  getFacets: (query, search, signal) => {
    const params = apiQueryParams(query)
    if (query.tab !== 'repo') params.delete('tab')
    for (const key of ['locationGroup', 'sort', 'direction', 'page', 'pageSize']) params.delete(key)
    params.set('search', search)
    return request(`/api/v2/usage/facets?${params}`, facetsSchema, signal)
  },
  getCollectorProgress: (signal) =>
    request('/api/v2/collector-progress', collectorProgressSchema, signal),
  getConnection: (signal) => request('/api/viewer/connection', viewerConnectionSchema, signal),
  refresh: async (signal) => {
    await send('/api/viewer/refresh', { method: 'POST', body: {}, signal })
  },
}
