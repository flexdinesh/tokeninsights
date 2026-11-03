import { createContext, useContext } from 'react'
import type { ReactNode } from 'react'
import type { DashboardClient } from '@tokeninsights/contracts'

const ClientContext = createContext<DashboardClient | undefined>(undefined)

export function DashboardClientProvider({
  client,
  children,
}: {
  client: DashboardClient
  children: ReactNode
}) {
  return <ClientContext.Provider value={client}>{children}</ClientContext.Provider>
}

export function useDashboardClient(): DashboardClient {
  const client = useContext(ClientContext)
  if (!client) throw new Error('DashboardClientProvider missing')
  return client
}
