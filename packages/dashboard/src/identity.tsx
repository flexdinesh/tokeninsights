import { createContext, useContext } from 'react'
import type { Bootstrap, ViewerConnection } from '@tokeninsights/contracts'

export const IdentityContext = createContext<
  | {
      bootstrap: Bootstrap
      connectionError: Error | null
      retryConnection: () => Promise<unknown>
      connection: ViewerConnection | undefined
    }
  | undefined
>(undefined)

export function useIdentity() {
  const identity = useContext(IdentityContext)
  if (!identity) throw new Error('Identity provider missing')
  return identity
}
