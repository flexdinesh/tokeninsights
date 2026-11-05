import type { Bootstrap } from '@tokeninsights/contracts'

export type Capability =
  | 'usage'
  | 'facets'
  | 'web-dashboard'
  | 'terminal-dashboard'
  | 'collector-progress'

export function hasCapability(server: Bootstrap, capability: Capability): boolean {
  return server.capabilities.includes(capability)
}

export function showsCollectorProgress(server: Bootstrap): boolean {
  return server.permissions.includes('read') && hasCapability(server, 'collector-progress')
}
