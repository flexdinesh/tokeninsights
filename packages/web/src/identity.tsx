import { createContext, useContext } from 'react'
import type { Bootstrap } from './contracts'

export const IdentityContext = createContext<
  | {
      bootstrap: Bootstrap
      connectionError: Error | null
      retryConnection: () => Promise<unknown>
      onSignOut: () => Promise<void>
      onSessionEnded: (message?: string) => Promise<void>
    }
  | undefined
>(undefined)

export function useIdentity() {
  const identity = useContext(IdentityContext)
  if (!identity) throw new Error('Identity provider missing')
  return identity
}
