import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useDashboardClient } from './client'
import type { QueryState } from './query'

const dashboardPollIntervalMs = 5_000

export function useBootstrap(enabled = true) {
  const api = useDashboardClient()
  return useQuery({
    queryKey: ['instance'],
    enabled,
    retry: false,
    queryFn: ({ signal }) => api.getInstance(signal),
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
}

export function useSyncStatus(enabled = true, datasetId?: string) {
  const api = useDashboardClient()
  return useQuery({
    queryKey: datasetId === undefined ? ['sync'] : ['sync', datasetId],
    queryFn: ({ signal }) => api.getStatus(signal),
    enabled,
    refetchInterval: dashboardPollIntervalMs,
  })
}

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
  const api = useDashboardClient()
  const summaryScope = JSON.stringify({
    period: q.period,
    quality: q.quality,
    from: q.from,
    to: q.to,
    providers: q.providers,
    models: q.models,
    harnesses: q.harnesses,
    sessions: q.sessions,
    locationGroup: q.tab === 'repo' ? q.locationGroup : undefined,
    repositories: q.tab === 'repo' ? q.repositories : [],
    directories: q.tab === 'repo' ? q.directories : [],
  })
  return useQuery({
    queryKey: ['usage', summaryScope, q, revision, identity],
    queryFn: async ({ signal }) => {
      const dashboard = await api.getUsage(q, signal)
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
      return previousKey?.[1] === summaryScope && previousKey?.[4] === identity
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
  const api = useDashboardClient()
  return useQuery({
    queryKey: ['facets', q, search, revision, identity],
    queryFn: async ({ signal }) => {
      const facets = await api.getFacets(q, search, signal)
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

export function useCollectorProgress(enabled: boolean, instanceId: string) {
  const api = useDashboardClient()
  return useQuery({
    queryKey: ['collector-progress', instanceId],
    queryFn: async ({ signal }) => {
      const progress = await api.getCollectorProgress(signal)
      if (progress.instanceId !== instanceId)
        throw new Error('Collector progress belongs to another server instance.')
      return progress
    },
    enabled,
    refetchInterval: dashboardPollIntervalMs,
    refetchOnWindowFocus: false,
  })
}
