import { skipToken, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { Dashboard } from '@tokeninsights/contracts'
import { useDashboardClient } from './client'
import type { QueryState } from './query'

const dashboardPollIntervalMs = 5_000

type AnalyticsSnapshot = {
  dashboard: Dashboard
  tab: QueryState['tab']
  locationGroup: QueryState['locationGroup']
}

export function useConnection(enabled = true) {
  const api = useDashboardClient()
  return useQuery({
    queryKey: ['viewer-connection'],
    queryFn: ({ signal }) => api.getConnection(signal),
    enabled,
    retry: false,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
}

export function useRefresh() {
  const api = useDashboardClient()
  const client = useQueryClient()
  return useMutation({
    mutationFn: () => api.refresh(),
    retry: false,
    onSettled: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: ['collector-progress'] }),
        client.invalidateQueries({ queryKey: ['sync'] }),
      ])
    },
  })
}

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
  // A subscribed saved snapshot survives failed publication reads while this
  // selection is open. Other selections/backends use separate cache entries.
  const savedKey = ['saved-usage', summaryScope, identity]
  const saved = useQuery<AnalyticsSnapshot>({ queryKey: savedKey, queryFn: skipToken })
  const result = useQuery({
    queryKey: ['usage', summaryScope, q, revision, identity],
    queryFn: async ({ signal }) => {
      const dashboard = await api.getUsage(q, signal)
      if (snapshotIdentity(dashboard) !== identity || dashboard.revision < revision) {
        void client.invalidateQueries({ queryKey: ['sync'] })
        throw new Error('Data changed. Refreshing service status.')
      }
      if (dashboard.revision > revision) void client.invalidateQueries({ queryKey: ['sync'] })
      const snapshot = { dashboard, tab: q.tab, locationGroup: q.locationGroup }
      client.setQueryData(savedKey, snapshot)
      return snapshot
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
  // A failed query for a new published revision must keep the last saved result
  // for this selection and backend. Changed selections never borrow old totals.
  const retained = saved.data
  return {
    ...result,
    data: result.data ?? retained,
    isPlaceholderData: result.isPlaceholderData || (!result.data && Boolean(retained)),
  }
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
