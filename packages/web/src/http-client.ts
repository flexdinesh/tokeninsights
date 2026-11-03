import {
  accountUserSchema,
  apiKeyCreatedSchema,
  apiKeyListSchema,
  authInfoSchema,
  bootstrapSchema,
  collectorProgressSchema,
  dashboardSchema,
  facetsSchema,
  statusSchema,
} from '@tokeninsights/contracts'
import type { DashboardClient } from '@tokeninsights/contracts'
import { parseBody, request, send } from './http'
import { apiQueryParams } from './http-query'

// Same-origin transport. Dataset headers bind account operations to the observed
// session; they never select or authorize another dataset.
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
  getAuthInfo: (signal) => request('/api/v2/auth', authInfoSchema, signal),
  getAccount: (datasetId, signal) =>
    request('/api/v2/account', accountUserSchema, signal, datasetId),
  getAPIKeys: (datasetId, signal) =>
    request('/api/v2/account/keys', apiKeyListSchema, signal, datasetId),
  changeBrowserSession: async (credentials) => {
    await send('/api/v2/auth/session', {
      method: credentials === undefined ? 'DELETE' : 'POST',
      body: credentials,
      failure: 'Sign in failed',
    })
  },
  signup: async (credentials) => {
    await send('/api/v2/auth/signup', { method: 'POST', body: credentials })
  },
  changePassword: async (datasetId, currentPassword, password) => {
    await send('/api/v2/account/password', {
      method: 'PUT',
      body: { currentPassword, password },
      datasetId,
    })
  },
  createAPIKey: async (datasetId, body) =>
    parseBody(
      await send('/api/v2/account/keys', { method: 'POST', body, datasetId }),
      apiKeyCreatedSchema,
    ),
  revokeAPIKey: async (datasetId, tokenId) => {
    await send(`/api/v2/account/keys/${encodeURIComponent(tokenId)}`, {
      method: 'DELETE',
      datasetId,
    })
  },
}
