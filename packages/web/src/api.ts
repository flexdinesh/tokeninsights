import { useQuery } from '@tanstack/react-query'
import { z } from 'zod'
import {
  bootstrapSchema,
  dashboardSchema,
  errorSchema,
  facetsSchema,
  statusSchema,
} from './contracts'
import { normalizeBaseUrl } from './sources'
import { queryParams } from './state'
import type { Bootstrap } from './contracts'
import type { QueryState } from './state'

function endpoint(baseUrl: string, path: string): string {
  return `${normalizeBaseUrl(baseUrl)}${path}`
}

export async function request<T>(
  baseUrl: string,
  path: string,
  schema: z.ZodType<T>,
  signal?: AbortSignal,
  method = 'GET',
): Promise<T> {
  const response = await fetch(endpoint(baseUrl, path), { signal, method })
  let body: unknown
  try {
    body = await response.json()
  } catch {
    throw new Error(`Invalid response from ${new URL(baseUrl).hostname}`)
  }
  if (!response.ok) {
    const error = errorSchema.safeParse(body)
    throw new Error(error.success ? error.data.message : `Request failed (${response.status})`)
  }
  return schema.parse(body)
}

export function getInstance(baseUrl: string, signal?: AbortSignal): Promise<Bootstrap> {
  return request(baseUrl, '/api/v1/instance', bootstrapSchema, signal)
}

export const useBootstrap = (baseUrl: string) =>
  useQuery({
    queryKey: ['instance', baseUrl],
    queryFn: ({ signal }) => getInstance(baseUrl, signal),
    staleTime: Infinity,
  })

export const useSyncStatus = (baseUrl: string, enabled = true) =>
  useQuery({
    queryKey: ['sync', baseUrl],
    queryFn: ({ signal }) => request(baseUrl, '/api/v1/sync', statusSchema, signal),
    enabled,
    refetchInterval: (query) => (query.state.data?.running ? 1000 : 5000),
  })

export const syncNow = (baseUrl: string) =>
  request(baseUrl, '/api/v1/sync', statusSchema, undefined, 'POST')

export function useAnalytics(baseUrl: string, q: QueryState, revision: number, enabled: boolean) {
  const params = queryParams(q)
  params.delete('v')
  return useQuery({
    queryKey: ['usage', baseUrl, params.toString(), revision],
    queryFn: ({ signal }) => request(baseUrl, `/api/v1/usage?${params}`, dashboardSchema, signal),
    enabled,
  })
}

export function useFacets(
  baseUrl: string,
  q: QueryState,
  revision: number,
  enabled: boolean,
  search = '',
) {
  const params = queryParams(q)
  params.delete('v')
  params.delete('tab')
  params.delete('sort')
  params.delete('direction')
  params.delete('page')
  params.delete('pageSize')
  params.set('search', search)
  return useQuery({
    queryKey: ['facets', baseUrl, params.toString(), revision],
    queryFn: ({ signal }) =>
      request(baseUrl, `/api/v1/usage/facets?${params}`, facetsSchema, signal),
    enabled,
  })
}
