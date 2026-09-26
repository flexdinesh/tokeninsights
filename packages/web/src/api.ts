import { useQuery } from '@tanstack/react-query'
import { z } from 'zod'
import {
  bootstrapSchema,
  dashboardSchema,
  errorSchema,
  facetsSchema,
  statusSchema,
} from './contracts'
import { apiQueryParams } from './state'
import type { Bootstrap } from './contracts'
import type { QueryState } from './state'

export async function request<T>(
  path: string,
  schema: z.ZodType<T>,
  signal?: AbortSignal,
  method = 'GET',
): Promise<T> {
  const response = await fetch(path, { signal, method })
  let body: unknown
  try {
    body = await response.json()
  } catch {
    throw new Error('Invalid response from server')
  }
  if (!response.ok) {
    const error = errorSchema.safeParse(body)
    throw new Error(error.success ? error.data.message : `Request failed (${response.status})`)
  }
  return schema.parse(body)
}

export function getInstance(signal?: AbortSignal): Promise<Bootstrap> {
  return request('/api/v1/instance', bootstrapSchema, signal)
}

export const useBootstrap = () =>
  useQuery({
    queryKey: ['instance'],
    queryFn: ({ signal }) => getInstance(signal),
    staleTime: Infinity,
  })

export const useSyncStatus = (enabled = true) =>
  useQuery({
    queryKey: ['sync'],
    queryFn: ({ signal }) => request('/api/v1/sync', statusSchema, signal),
    enabled,
    refetchInterval: (query) => (query.state.data?.running ? 1000 : 5000),
  })

export const syncNow = () => request('/api/v1/sync', statusSchema, undefined, 'POST')

export function useAnalytics(q: QueryState, revision: number, enabled: boolean, running = false) {
  const params = apiQueryParams(q)
  const summaryScope = apiQueryParams(q)
  for (const key of ['bucket', 'tab', 'sort', 'direction', 'page', 'pageSize']) {
    summaryScope.delete(key)
  }
  return useQuery({
    refetchInterval: running ? 1000 : false,
    queryKey: ['usage', summaryScope.toString(), params.toString(), revision],
    queryFn: async ({ signal }) => ({
      dashboard: await request(`/api/v1/usage?${params}`, dashboardSchema, signal),
      tab: q.tab,
      locationGroup: q.locationGroup,
    }),
    enabled,
    placeholderData: (previous, previousQuery) => {
      const previousKey = previousQuery?.queryKey
      return previousKey?.[1] === summaryScope.toString() ? previous : undefined
    },
  })
}

export function useFacets(q: QueryState, revision: number, enabled: boolean, search = '') {
  const params = apiQueryParams(q)
  if (q.tab !== 'repo') params.delete('tab')
  params.delete('locationGroup')
  params.delete('sort')
  params.delete('direction')
  params.delete('page')
  params.delete('pageSize')
  params.set('search', search)
  return useQuery({
    queryKey: ['facets', params.toString(), revision],
    queryFn: ({ signal }) => request(`/api/v1/usage/facets?${params}`, facetsSchema, signal),
    enabled,
  })
}
