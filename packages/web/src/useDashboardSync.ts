import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { snapshotIdentity, useCollectorProgress, useSyncStatus } from './api'

export function useDashboardSync(
  datasetId?: string,
  progress?: { enabled: boolean; instanceId: string },
) {
  const client = useQueryClient()
  const statusQuery = useSyncStatus(true, datasetId)
  const progressEnabled = progress?.enabled ?? false
  const progressQuery = useCollectorProgress(progressEnabled, progress?.instanceId ?? '')
  const [reloading, setReloading] = useState(false)
  const reloadPending = useRef(false)
  const status = statusQuery.data
  const revision = status?.revision ?? 0
  const identity = status ? snapshotIdentity(status) : ''
  const previousIdentity = useRef(identity)
  const previousRevision = useRef(revision)
  const attempts = progressEnabled ? (progressQuery.data?.attempts ?? []) : []
  const activeAttempt = attempts.find((attempt) =>
    ['waiting', 'capturing', 'submitting'].includes(attempt.stage),
  )
  const latestAttempt = attempts.reduce<(typeof attempts)[number] | undefined>(
    (latest, attempt) => (!latest || attempt.updatedAtMs >= latest.updatedAtMs ? attempt : latest),
    undefined,
  )
  const acceptedTransition =
    latestAttempt?.stage === 'accepted'
      ? `${latestAttempt.attemptId}/${latestAttempt.updatedAtMs}`
      : ''
  const [checkedAcceptance, setCheckedAcceptance] = useState('')
  const checkingAcceptance = acceptedTransition !== '' && acceptedTransition !== checkedAcceptance
  const refetchStatus = statusQuery.refetch
  useEffect(() => {
    if (!checkingAcceptance) return undefined
    let current = true
    void refetchStatus().then(() => {
      if (current) setCheckedAcceptance(acceptedTransition)
    })
    return () => {
      current = false
    }
  }, [acceptedTransition, checkingAcceptance, refetchStatus])

  const collectionUnknown = progressEnabled && progressQuery.isError
  const collectionPending =
    progressEnabled &&
    !collectionUnknown &&
    (progressQuery.isPending || Boolean(activeAttempt) || checkingAcceptance)
  const collectionFailed =
    latestAttempt?.stage === 'failed' || latestAttempt?.stage === 'interrupted'
  const processingPending = Boolean(
    status && (status.pending > 0 || status.generation !== status.targetGeneration),
  )
  useEffect(() => {
    if (previousIdentity.current !== identity) {
      previousIdentity.current = identity
      for (const key of ['usage', 'facets']) {
        void client.cancelQueries({ queryKey: [key] })
        client.removeQueries({ queryKey: [key] })
      }
      void client.invalidateQueries({ queryKey: ['instance'] })
    }
    if (previousRevision.current === revision) return
    previousRevision.current = revision
    for (const key of ['usage', 'facets', 'instance']) {
      void client.invalidateQueries({ queryKey: [key] })
    }
  }, [client, revision, identity])

  async function reload() {
    if (reloadPending.current) return
    reloadPending.current = true
    setReloading(true)
    try {
      // Cancel older reads before obtaining the current database identity/revision.
      await client.cancelQueries({ queryKey: ['sync'] })
      await client.invalidateQueries({ queryKey: ['sync'] })
      await client.invalidateQueries({ queryKey: ['instance'] })
      await Promise.all(
        ['usage', 'facets'].map((key) => client.invalidateQueries({ queryKey: [key] })),
      )
    } finally {
      reloadPending.current = false
      setReloading(false)
    }
  }
  return {
    statusQuery,
    progressQuery,
    collectionStage: collectionUnknown ? undefined : activeAttempt?.stage,
    collectionPending,
    collectionUnknown,
    collectionFailed,
    processingPending,
    processingBackoff:
      (status?.failed ?? 0) > 0 && (status?.failedRetryAtMs ?? 0) > statusQuery.dataUpdatedAt,
    loading: collectionPending || processingPending || !status,
    reload,
    reloading,
    revision,
    identity,
    analyticsEnabled:
      status?.dataReadiness === 'ready' &&
      (datasetId === undefined || status.datasetId === datasetId),
  }
}
