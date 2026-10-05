const compact = new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 })
const exact = new Intl.NumberFormat('en')
export const formatCount = (n: number) => compact.format(n)
export const exactCount = (n: number) => exact.format(n)

const millisecondsPerMinute = 60_000

function serverZone(timezone: string) {
  const offset = /^UTC([+-])(\d{2}):(\d{2})$/.exec(timezone)
  if (offset) {
    const hours = Number(offset[2])
    const minutes = Number(offset[3])
    if (hours <= 23 && minutes <= 59) {
      return {
        timeZone: 'UTC',
        offsetMinutes: (hours * 60 + minutes) * (offset[1] === '-' ? -1 : 1),
        label: timezone,
      }
    }
  } else if (timezone) {
    try {
      new Intl.DateTimeFormat('en', { timeZone: timezone }).format(0)
      return { timeZone: timezone, offsetMinutes: 0, label: timezone }
    } catch {
      // Unknown server labels never silently adopt the browser's timezone.
    }
  }
  return { timeZone: 'UTC', offsetMinutes: 0, label: 'UTC (server timezone unavailable)' }
}

export function serverTimeZoneLabel(timezone: string): string {
  return serverZone(timezone).label
}

function formatServerTime(milliseconds: number, timezone: string, includeTime: boolean): string {
  const zone = serverZone(timezone)
  return new Intl.DateTimeFormat('en', {
    timeZone: zone.timeZone,
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    ...(includeTime ? { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' } : {}),
  }).format(milliseconds + zone.offsetMinutes * millisecondsPerMinute)
}

export const formatServerDate = (milliseconds: number, timezone: string) =>
  formatServerTime(milliseconds, timezone, false)
export const formatServerDateTime = (milliseconds: number, timezone: string) =>
  formatServerTime(milliseconds, timezone, true)
export const labels = {
  tokens: 'Tokens',
  models: 'Models',
  providers: 'Providers',
  harnesses: 'Harnesses',
  sessions: 'Sessions',
  context: 'Context',
  repo: 'Repo',
}
