import type { Bootstrap } from './contracts'

export type Capability =
  | 'usage'
  | 'facets'
  | 'web-dashboard'
  | 'raw-ingestion'
  | 'terminal-dashboard'
  | 'collector-progress'
  | 'dashboard-reload'
  | 'reprocess'

export function hasCapability(server: Bootstrap, capability: Capability): boolean {
  return server.capabilities.includes(capability)
}

export function showsCollectorProgress(server: Bootstrap): boolean {
  return (
    server.serverKind === 'personal' &&
    server.permissions.includes('read') &&
    hasCapability(server, 'collector-progress')
  )
}

export function allowsDashboardReload(server: Bootstrap): boolean {
  return (
    server.serverKind === 'personal' &&
    server.permissions.includes('read') &&
    hasCapability(server, 'dashboard-reload')
  )
}
