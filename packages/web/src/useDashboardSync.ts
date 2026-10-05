import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useSyncStatus } from './api'

export function useDashboardSync() {
  const client = useQueryClient()
  const statusQuery = useSyncStatus()
  const [reloading, setReloading] = useState(false)
  const reloadPending = useRef(false)
  const status = statusQuery.data
  const revision = status?.revision ?? 0
  const identity =
    status?.instanceId && status.dataEpoch ? `${status.instanceId}/${status.dataEpoch}` : ''
  const previousIdentity = useRef(identity)
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
    reload,
    reloading,
    revision,
    identity,
    analyticsEnabled: Boolean(
      status && (!status.dataReadiness || status.dataReadiness === 'ready'),
    ),
  }
}
