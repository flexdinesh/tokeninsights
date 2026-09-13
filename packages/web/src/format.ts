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
