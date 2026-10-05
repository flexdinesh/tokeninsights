import { expect, it } from 'vitest'
import { formatServerDate, formatServerDateTime, serverTimeZoneLabel } from './format'

it('uses the server calendar across a browser/server midnight boundary', () => {
  const instant = Date.parse('2026-01-01T00:30:00Z')
  expect(formatServerDate(instant, 'UTC')).toBe('Jan 1, 2026')
  expect(formatServerDate(instant, 'America/Los_Angeles')).toBe('Dec 31, 2025')
})

it('uses the historical DST offset of an IANA server timezone', () => {
  expect(formatServerDateTime(Date.parse('2026-01-15T00:00:00Z'), 'Australia/Sydney')).toBe(
    'Jan 15, 2026, 11:00',
  )
  expect(formatServerDateTime(Date.parse('2026-07-15T00:00:00Z'), 'Australia/Sydney')).toBe(
    'Jul 15, 2026, 10:00',
  )
})

it('supports explicit fixed offsets without interpreting them as IANA DST zones', () => {
  const instant = Date.parse('2026-01-01T00:30:00Z')
  expect(formatServerDateTime(instant, 'UTC-03:30')).toBe('Dec 31, 2025, 21:00')
  expect(formatServerDateTime(instant, 'UTC+05:45')).toBe('Jan 1, 2026, 06:15')
  expect(serverTimeZoneLabel('UTC+05:45')).toBe('UTC+05:45')
})

it('labels unavailable server zones and falls back explicitly to UTC', () => {
  const instant = Date.parse('2026-01-01T00:30:00Z')
  for (const timezone of ['', 'invalid-zone', 'UTC+99:00']) {
    expect(serverTimeZoneLabel(timezone)).toBe('UTC (server timezone unavailable)')
    expect(formatServerDateTime(instant, timezone)).toBe('Jan 1, 2026, 00:30')
  }
})
