import { z } from 'zod'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { bootstrapSchema, dashboardSchema, facetsSchema, statusSchema } from './contracts'
import { queryParams } from './state'
import type { QueryState } from './state'

export async function request<T>(path: string, schema: z.ZodType<T>, signal?: AbortSignal, method = 'GET'): Promise<T> {
  const response = await fetch(path, { signal, method })
  const body: unknown = await response.json()
  if (!response.ok) {
    const error = z.object({ error: z.string() }).safeParse(body)
    throw new Error(error.success ? error.data.error : `Request failed (${response.status})`)
  }
  return schema.parse(body)
}

export const useBootstrap = () => useQuery({ queryKey: ['bootstrap'], queryFn: ({ signal }) => request('/api/bootstrap', bootstrapSchema, signal), staleTime: Infinity })
export const useSyncStatus = () => useQuery({ queryKey: ['status'], queryFn: ({ signal }) => request('/api/status', statusSchema, signal), refetchInterval: query => query.state.data?.running ? 1000 : 5000 })
export const syncNow = () => request('/api/sync', statusSchema, undefined, 'POST')

export function useAnalytics(q: QueryState, revision: number, enabled: boolean) {
  return useQuery({ queryKey: ['dashboard', queryParams(q).toString(), revision], queryFn: ({ signal }) => request(`/api/dashboard?${queryParams(q)}`, dashboardSchema, signal), enabled, placeholderData: keepPreviousData })
}

export function useFacets(q: QueryState, revision: number, enabled: boolean, search = '') {
  const params = queryParams(q)
  params.set('search', search)
  return useQuery({ queryKey: ['facets', params.toString(), revision], queryFn: ({ signal }) => request(`/api/filters?${params}`, facetsSchema, signal), enabled })
}
