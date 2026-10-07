import { useQuery, useQueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import {
  bootstrapSchema,
  dashboardSchema,
  collectorProgressSchema,
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
): Promise<T> {
  const response = await fetch(path, { signal, method: 'GET', credentials: 'same-origin' })
  if (response.status === 401) throw new AuthenticationRequired()
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
  return request('/api/v2/instance', bootstrapSchema, signal)
}

export const useBootstrap = (enabled = true) =>
  useQuery({
    queryKey: ['instance'],
    enabled,
    retry: false,
    queryFn: ({ signal }) => getInstance(signal),
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })

export const useSyncStatus = (enabled = true, datasetId?: string) =>
  useQuery({
    queryKey: datasetId === undefined ? ['sync'] : ['sync', datasetId],
    queryFn: ({ signal }) => request('/api/v2/status', statusSchema, signal),
    enabled,
    refetchInterval: 5000,
  })

export function snapshotIdentity(value: {
  instanceId: string
  dataEpoch: string
  generation?: number
  datasetId: string
}): string {
  return `${value.instanceId}/${value.dataEpoch}/${value.datasetId}${value.generation === undefined ? '' : `/${value.generation}`}`
}

export function useAnalytics(q: QueryState, revision: number, enabled: boolean, identity: string) {
  const client = useQueryClient()
  const params = apiQueryParams(q)
  const summaryScope = apiQueryParams(q)
  for (const key of ['bucket', 'tab', 'sort', 'direction', 'page', 'pageSize']) {
    summaryScope.delete(key)
  }
  return useQuery({
    queryKey: ['usage', summaryScope.toString(), params.toString(), revision, identity],
    queryFn: async ({ signal }) => {
      const dashboard = await request(`/api/v2/usage?${params}`, dashboardSchema, signal)
      if (snapshotIdentity(dashboard) !== identity || dashboard.revision < revision) {
        void client.invalidateQueries({ queryKey: ['sync'] })
        throw new Error('Data changed. Refreshing service status.')
      }
      if (dashboard.revision > revision) void client.invalidateQueries({ queryKey: ['sync'] })
      return { dashboard, tab: q.tab, locationGroup: q.locationGroup }
    },
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    enabled,
    placeholderData: (previous, previousQuery) => {
      const previousKey = previousQuery?.queryKey
      return previousKey?.[1] === summaryScope.toString() && previousKey?.[4] === identity
        ? previous
        : undefined
    },
  })
}

export function useFacets(
  q: QueryState,
  revision: number,
  enabled: boolean,
  identity: string,
  search = '',
) {
  const client = useQueryClient()
  const params = apiQueryParams(q)
  if (q.tab !== 'repo') params.delete('tab')
  params.delete('locationGroup')
  params.delete('sort')
  params.delete('direction')
  params.delete('page')
  params.delete('pageSize')
  params.set('search', search)
  return useQuery({
    queryKey: ['facets', params.toString(), revision, identity],
    queryFn: async ({ signal }) => {
      const facets = await request(`/api/v2/usage/facets?${params}`, facetsSchema, signal)
      if (snapshotIdentity(facets) !== identity || facets.revision < revision) {
        void client.invalidateQueries({ queryKey: ['sync'] })
        throw new Error('Data changed. Refreshing service status.')
      }
      if (facets.revision > revision) void client.invalidateQueries({ queryKey: ['sync'] })
      return facets
    },
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    enabled,
  })
}

export class AuthenticationRequired extends Error {
  constructor() {
    super('Sign in to view your token usage.')
    this.name = 'AuthenticationRequired'
  }
}

export async function changeBrowserSession(token?: string): Promise<void> {
  const response = await fetch('/api/v2/auth/session', {
    method: token === undefined ? 'DELETE' : 'POST',
    credentials: 'same-origin',
    headers: token === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: token === undefined ? undefined : JSON.stringify({ token }),
  })
  if (response.status === 401) throw new AuthenticationRequired()
  if (!response.ok) throw new Error(`Sign in failed (${response.status})`)
}

export const useCollectorProgress = (enabled: boolean, instanceId: string) =>
  useQuery({
    queryKey: ['collector-progress', instanceId],
    queryFn: async ({ signal }) => {
      const progress = await request('/api/v2/collector-progress', collectorProgressSchema, signal)
      if (progress.instanceId !== instanceId)
        throw new Error('Collector progress belongs to another server instance.')
      return progress
    },
    enabled,
    refetchInterval: 1000,
    refetchOnWindowFocus: false,
  })
