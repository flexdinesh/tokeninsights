import { serverZone } from './format'

export type CalendarPeriod = 'today' | 'yesterday' | 'week' | 'month' | 'year'

const daysPerWeek = 7
const monday = 1
const millisecondsPerMinute = 60_000
const dayFormatter = new Intl.DateTimeFormat('en', {
  timeZone: 'UTC',
  year: 'numeric',
  month: 'short',
  day: 'numeric',
})
const monthFormatter = new Intl.DateTimeFormat('en', {
  timeZone: 'UTC',
  year: 'numeric',
  month: 'long',
})

function isoDate(date: Date): string {
  return date.toISOString().slice(0, 10)
}

function rangeFromDate(period: CalendarPeriod, date: Date): { from: string; to: string } {
  const start = new Date(date)
  if (period === 'week') {
    start.setUTCDate(
      start.getUTCDate() - ((start.getUTCDay() - monday + daysPerWeek) % daysPerWeek),
    )
  } else if (period === 'month') {
    start.setUTCDate(1)
  } else if (period === 'year') {
    start.setUTCMonth(0, 1)
  }
  const end = new Date(start)
  if (period === 'week') {
    end.setUTCDate(end.getUTCDate() + daysPerWeek - 1)
  } else if (period === 'month') {
    end.setUTCMonth(end.getUTCMonth() + 1)
    end.setUTCDate(end.getUTCDate() - 1)
  } else if (period === 'year') {
    end.setUTCFullYear(end.getUTCFullYear() + 1)
    end.setUTCDate(end.getUTCDate() - 1)
  }
  return { from: isoDate(start), to: isoDate(end) }
}

export function isCalendarRange(period: string, from: string, to: string): boolean {
  if (
    period !== 'today' &&
    period !== 'yesterday' &&
    period !== 'week' &&
    period !== 'month' &&
    period !== 'year'
  ) {
    return false
  }
  if (!/^\d{4}-\d{2}-\d{2}$/.test(from) || !/^\d{4}-\d{2}-\d{2}$/.test(to) || from > to) {
    return false
  }
  const date = new Date(`${from}T00:00:00Z`)
  if (Number.isNaN(date.getTime()) || isoDate(date) !== from) return false
  const range = rangeFromDate(period, date)
  return range.from === from && range.to === to
}

export function calendarRange(
  period: CalendarPeriod,
  timezone: string,
  now: Date = new Date(),
): { from: string; to: string } {
  const zone = serverZone(timezone)
  const parts = new Intl.DateTimeFormat('en', {
    timeZone: zone.timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(now.getTime() + zone.offsetMinutes * millisecondsPerMinute)
  const part = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((value) => value.type === type)?.value
  // UTC carries calendar dates only. Elapsed hours never determine a day or week.
  const date = new Date(`${part('year')}-${part('month')}-${part('day')}T00:00:00Z`)
  if (period === 'yesterday') date.setUTCDate(date.getUTCDate() - 1)
  return rangeFromDate(period, date)
}

export function shiftCalendarRange(
  period: CalendarPeriod,
  from: string,
  direction: -1 | 1,
): { from: string; to: string } {
  const current = rangeFromDate(period, new Date(`${from}T00:00:00Z`))
  const date = new Date(`${current.from}T00:00:00Z`)
  if (period === 'year') {
    date.setUTCFullYear(date.getUTCFullYear() + direction)
  } else if (period === 'month') {
    date.setUTCMonth(date.getUTCMonth() + direction)
  } else {
    date.setUTCDate(date.getUTCDate() + direction * (period === 'week' ? daysPerWeek : 1))
  }
  return rangeFromDate(period, date)
}

export function calendarRangeLabel(period: CalendarPeriod, from: string, to: string): string {
  const start = new Date(`${from}T00:00:00Z`)
  if (period === 'year') return String(start.getUTCFullYear())
  if (period === 'month') return monthFormatter.format(start)
  if (period === 'week') {
    return dayFormatter.formatRange(start, new Date(`${to}T00:00:00Z`)).replace(/\s+/g, ' ')
  }
  return dayFormatter.format(start)
}
