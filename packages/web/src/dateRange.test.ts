import { describe, expect, it } from 'vitest'
import { calendarRange, calendarRangeLabel, isCalendarRange, shiftCalendarRange } from './dateRange'

it('accepts only complete aligned calendar ranges with real ordered ISO dates', () => {
  expect(isCalendarRange('today', '2024-02-29', '2024-02-29')).toBe(true)
  expect(isCalendarRange('yesterday', '2026-10-17', '2026-10-17')).toBe(true)
  expect(isCalendarRange('week', '2025-12-29', '2026-01-04')).toBe(true)
  expect(isCalendarRange('month', '2024-02-01', '2024-02-29')).toBe(true)
  expect(isCalendarRange('year', '2024-01-01', '2024-12-31')).toBe(true)

  const invalid: [string, string, string][] = [
    ['all', '2026-10-01', '2026-10-31'],
    ['unknown', '2026-10-01', '2026-10-31'],
    ['week', '2026-13-01', '2026-13-02'],
    ['week', '2026-10-18', '2026-10-12'],
    ['week', '2026-10-13', '2026-10-19'],
    ['week', '2026-10-12', '2026-10-17'],
    ['month', '2026-10-02', '2026-10-31'],
    ['month', '2026-10-01', '2026-11-01'],
    ['month', '2026-02-01', '2026-02-29'],
    ['year', '2026-01-01', '2026-12-30'],
    ['today', '2026-02-29', '2026-02-29'],
    ['today', '2026-10-18', '2026-10-19'],
    ['today', '2026-10-18', ''],
    ['today', '2026-1-01', '2026-1-01'],
    ['today', '', ''],
  ]
  for (const [period, from, to] of invalid) {
    expect(isCalendarRange(period, from, to)).toBe(false)
  }
})

describe('server calendar ranges', () => {
  it('resolves dates in the server timezone across midnight and year boundaries', () => {
    const now = new Date('2026-01-01T00:30:00Z')
    expect(calendarRange('today', 'UTC', now)).toEqual({ from: '2026-01-01', to: '2026-01-01' })
    expect(calendarRange('today', 'America/Los_Angeles', now)).toEqual({
      from: '2025-12-31',
      to: '2025-12-31',
    })
    expect(calendarRange('yesterday', 'UTC', now)).toEqual({ from: '2025-12-31', to: '2025-12-31' })
    expect(calendarRange('year', 'America/Los_Angeles', now)).toEqual({
      from: '2025-01-01',
      to: '2025-12-31',
    })
  })

  it('uses full Monday through Sunday bounds across DST transitions', () => {
    expect(calendarRange('week', 'Australia/Sydney', new Date('2026-10-04T12:00:00Z'))).toEqual({
      from: '2026-09-28',
      to: '2026-10-04',
    })
    expect(calendarRange('week', 'America/New_York', new Date('2026-11-01T18:00:00Z'))).toEqual({
      from: '2026-10-26',
      to: '2026-11-01',
    })
    expect(calendarRange('week', 'Australia/Sydney', new Date('2026-10-04T13:30:00Z'))).toEqual({
      from: '2026-10-05',
      to: '2026-10-11',
    })
  })

  it('includes the final calendar day of months and leap years', () => {
    const now = new Date('2024-02-29T12:00:00Z')
    expect(calendarRange('month', 'UTC', now)).toEqual({ from: '2024-02-01', to: '2024-02-29' })
    expect(calendarRange('year', 'UTC', now)).toEqual({ from: '2024-01-01', to: '2024-12-31' })
  })

  it('preserves the shared fixed-offset and unavailable-timezone policy', () => {
    const now = new Date('2026-01-01T00:30:00Z')
    expect(calendarRange('today', 'UTC-03:30', now)).toEqual({
      from: '2025-12-31',
      to: '2025-12-31',
    })
    expect(calendarRange('today', 'UTC+05:45', new Date('2025-12-31T19:00:00Z'))).toEqual({
      from: '2026-01-01',
      to: '2026-01-01',
    })
    for (const timezone of ['', 'invalid-zone', 'UTC+99:00']) {
      expect(calendarRange('today', timezone, now)).toEqual({
        from: '2026-01-01',
        to: '2026-01-01',
      })
    }
  })
})

describe('calendar navigation', () => {
  it('moves daily and legacy yesterday one calendar day in either direction', () => {
    expect(shiftCalendarRange('today', '2024-03-01', -1)).toEqual({
      from: '2024-02-29',
      to: '2024-02-29',
    })
    expect(shiftCalendarRange('yesterday', '2024-02-29', 1)).toEqual({
      from: '2024-03-01',
      to: '2024-03-01',
    })
    expect(shiftCalendarRange('today', '2025-12-31', 1)).toEqual({
      from: '2026-01-01',
      to: '2026-01-01',
    })
  })

  it('moves full weeks across DST and year boundaries', () => {
    expect(shiftCalendarRange('week', '2026-09-28', 1)).toEqual({
      from: '2026-10-05',
      to: '2026-10-11',
    })
    expect(shiftCalendarRange('week', '2026-10-05', -1)).toEqual({
      from: '2026-09-28',
      to: '2026-10-04',
    })
    expect(shiftCalendarRange('week', '2025-12-29', 1)).toEqual({
      from: '2026-01-05',
      to: '2026-01-11',
    })
  })

  it('moves whole months without overflowing shorter or leap months', () => {
    expect(shiftCalendarRange('month', '2024-03-01', -1)).toEqual({
      from: '2024-02-01',
      to: '2024-02-29',
    })
    expect(shiftCalendarRange('month', '2026-01-01', -1)).toEqual({
      from: '2025-12-01',
      to: '2025-12-31',
    })
    expect(shiftCalendarRange('month', '2025-12-01', 1)).toEqual({
      from: '2026-01-01',
      to: '2026-01-31',
    })
    expect(shiftCalendarRange('month', '2024-02-01', 1)).toEqual({
      from: '2024-03-01',
      to: '2024-03-31',
    })
  })

  it('moves whole years in either direction through leap years', () => {
    expect(shiftCalendarRange('year', '2024-01-01', 1)).toEqual({
      from: '2025-01-01',
      to: '2025-12-31',
    })
    expect(shiftCalendarRange('year', '2025-01-01', -1)).toEqual({
      from: '2024-01-01',
      to: '2024-12-31',
    })
  })
})

it('labels the visible calendar range without using the browser timezone', () => {
  expect(calendarRangeLabel('today', '2026-10-18', '2026-10-18')).toBe('Oct 18, 2026')
  expect(calendarRangeLabel('yesterday', '2026-10-17', '2026-10-17')).toBe('Oct 17, 2026')
  expect(calendarRangeLabel('week', '2026-10-12', '2026-10-18')).toBe('Oct 12 – 18, 2026')
  expect(calendarRangeLabel('week', '2026-09-28', '2026-10-04')).toBe('Sep 28 – Oct 4, 2026')
  expect(calendarRangeLabel('week', '2025-12-29', '2026-01-04')).toBe('Dec 29, 2025 – Jan 4, 2026')
  expect(calendarRangeLabel('month', '2026-10-01', '2026-10-31')).toBe('October 2026')
  expect(calendarRangeLabel('year', '2026-01-01', '2026-12-31')).toBe('2026')
})
