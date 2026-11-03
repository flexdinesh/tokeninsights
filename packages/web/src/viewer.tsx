import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import type { Bootstrap } from '@tokeninsights/contracts'

// Browser tabs own only their own viewer. EventSource reconnects during backend
// grace; closing the tab releases its stream. Hosted instances never attach.
export function BrowserViewer() {
  const client = useQueryClient()
  useEffect(() => {
    let viewer: EventSource | undefined
    let instanceId: string | undefined
    const update = () => {
      // Observe the app's successful bootstrap without enabling another query.
      // Its authentication gate remains the only owner of instance requests.
      const instance = client.getQueryData<Bootstrap>(['instance'])
      const next = instance?.serverKind === 'personal' ? instance.instanceId : undefined
      if (next === instanceId) return
      viewer?.close()
      instanceId = next
      viewer = next === undefined ? undefined : new EventSource('/api/local/v1/viewers')
    }
    update()
    const unsubscribe = client.getQueryCache().subscribe((event) => {
      if (event.query.queryKey[0] === 'instance') update()
    })
    return () => {
      unsubscribe()
      viewer?.close()
    }
  }, [client])
  return null
}
