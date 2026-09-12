const compact = new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 })
const exact = new Intl.NumberFormat('en')
export const formatCount = (n: number) => compact.format(n)
export const exactCount = (n: number) => exact.format(n)
export const labels = {
  tokens: 'Tokens',
  models: 'Models',
  providers: 'Providers',
  harnesses: 'Harnesses',
  sessions: 'Sessions',
  context: 'Context',
}
export function relativeTime(timestamp: number): string {
  if (!timestamp) return 'Never synced'
  const minutes = Math.max(0, Math.floor((Date.now() - timestamp) / 60000))
  if (minutes === 0) return 'Synced just now'
  if (minutes < 60) return `Synced ${minutes}m ago`
  if (minutes < 1440) return `Synced ${Math.floor(minutes / 60)}h ago`
  return `Synced ${Math.floor(minutes / 1440)}d ago`
}
