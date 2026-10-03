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
  const identity =
    status?.instanceId && status.dataEpoch ? `${status.instanceId}/${status.dataEpoch}` : ''
  const previousIdentity = useRef(identity)
  const progressStamp = `${status?.progress?.jobId ?? 0}/${status?.progress?.updatedAt ?? 0}`
  const previousProgress = useRef(progressStamp)
  const previousRevision = useRef(revision)
  useEffect(() => {
    if (previousIdentity.current !== identity) {
      previousIdentity.current = identity
      for (const key of ['usage', 'facets']) {
        void client.cancelQueries({ queryKey: [key] })
        client.removeQueries({ queryKey: [key] })
      }
      void client.invalidateQueries({ queryKey: ['instance'] })
    }
    if (previousProgress.current !== progressStamp) {
      previousProgress.current = progressStamp
      void client.invalidateQueries({ queryKey: ['usage'] })
    }
    if (previousRevision.current === revision) return
    previousRevision.current = revision
    void client.invalidateQueries({ queryKey: ['instance'] })
  }, [client, revision, identity, progressStamp])

  const startSync = () => {
    if (submitted.current || status?.pendingRefresh) return
    submitted.current = true
    sync.mutate(undefined, {
      onSettled: () => {
        submitted.current = false
      },
    })
  }
  const reload = useCallback(async () => {
    await client.invalidateQueries({ queryKey: ['sync'] })
    await client.invalidateQueries({ queryKey: ['instance'] })
    for (const key of ['usage', 'facets']) {
      void client.invalidateQueries({ queryKey: [key] })
    }
  }, [client])
  return {
    statusQuery,
    sync,
    startSync,
    reload,
    revision,
    identity,
    pendingRefresh: Boolean(status?.pendingRefresh),
    refreshDisabled: sync.isPending || Boolean(status?.pendingRefresh),
    running: status?.running || sync.isPending,
    analyticsEnabled: Boolean(
      status &&
      (!status.dataReadiness || status.dataReadiness === 'ready') &&
      !['resetting', 'rebuilding', 'rebuild_failed'].includes(status.phase),
    ),
    checkedSince:
      !status || sync.isPending
        ? undefined
        : status.checkRequestedAt || status.progress?.startedAt || (status.running ? undefined : 0),
  }
}
