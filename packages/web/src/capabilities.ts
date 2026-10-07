import type { Bootstrap } from './contracts'

export type Capability =
  | 'usage'
  | 'facets'
  | 'web-dashboard'
  | 'raw-ingestion'
  | 'terminal-dashboard'
  | 'collector-progress'
  | 'reprocess'

export function hasCapability(server: Bootstrap, capability: Capability): boolean {
  return server.capabilities.includes(capability)
}

export function showsCollectorProgress(server: Bootstrap): boolean {
  return server.serverKind === 'personal' && hasCapability(server, 'collector-progress')
}
