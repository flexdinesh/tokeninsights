import { useCallback, useEffect, useRef } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { syncNow, useSyncStatus } from './api'

export function useDashboardSync() {
  const client = useQueryClient()
  const submitted = useRef(false)
  const sync = useMutation({
    mutationFn: syncNow,
    onMutate: () => client.cancelQueries({ queryKey: ['sync'] }),
    onSuccess: async (value) => {
      // A poll may have started before the disabled observer committed.
      await client.cancelQueries({ queryKey: ['sync'] })
      client.setQueryData(['sync'], value)
    },
    onSettled: () => client.invalidateQueries({ queryKey: ['sync'] }),
  })
  const statusQuery = useSyncStatus(!sync.isPending)
  const status = statusQuery.data
  const revision = status?.revision ?? 0
  const previousRevision = useRef(revision)
  useEffect(() => {
    if (previousRevision.current === revision) return
    previousRevision.current = revision
    void client.invalidateQueries({ queryKey: ['instance'] })
  }, [client, revision])

  const startSync = () => {
    if (submitted.current || status?.running) return
    submitted.current = true
    sync.mutate(undefined, {
      onSettled: () => {
        submitted.current = false
      },
    })
  }
  const reload = useCallback(() => {
    for (const key of ['usage', 'facets', 'sync', 'instance']) {
      void client.invalidateQueries({ queryKey: [key] })
    }
  }, [client])
  return {
    statusQuery,
    sync,
    startSync,
    reload,
    revision,
    running: status?.running || sync.isPending,
    analyticsEnabled: Boolean(
      status && !['resetting', 'rebuilding', 'rebuild_failed'].includes(status.phase),
    ),
    checkedSince:
      !status || sync.isPending
        ? undefined
        : status.progress?.startedAt || (status.running ? undefined : 0),
  }
}
