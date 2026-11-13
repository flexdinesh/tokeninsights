import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { expect, it } from 'vitest'
import { z } from 'zod'
import {
  calendarRange,
  calendarRangeLabel,
  isCalendarRange,
  shiftCalendarRange,
} from '../../dashboard/src/dateRange'

// Shared with packages/core/dashboardview so native calendar navigation stays identical.
const calendarPeriod = z.enum(['today', 'yesterday', 'week', 'month', 'year'])
const range = z.strictObject({ from: z.string(), to: z.string() })
const fixture = z
  .strictObject({
    isCalendarRange: z.array(
      z.strictObject({
        period: z.string(),
        from: z.string(),
        to: z.string(),
        expected: z.boolean(),
      }),
    ),
    calendarRange: z.array(
      z.strictObject({
        period: calendarPeriod,
        timezone: z.string(),
        now: z.iso.datetime(),
        expected: range,
      }),
    ),
    shiftCalendarRange: z.array(
      z.strictObject({
        period: calendarPeriod,
        from: z.string(),
        direction: z.union([z.literal(-1), z.literal(1)]),
        expected: range,
      }),
    ),
    calendarRangeLabel: z.array(
      z.strictObject({
        period: calendarPeriod,
        from: z.string(),
        to: z.string(),
        expected: z.string(),
      }),
    ),
  })
  .parse(
    JSON.parse(
      readFileSync(
        join(import.meta.dirname, '../../contracts/fixtures/dashboard-view/date-range.json'),
        'utf8',
      ),
    ),
  )

it('accepts only complete aligned calendar ranges with real ordered ISO dates', () => {
  for (const { period, from, to, expected } of fixture.isCalendarRange) {
    expect(isCalendarRange(period, from, to), `${period} ${from} ${to}`).toBe(expected)
  }
})

it('resolves Monday-first ranges on the server calendar across midnight, DST and fallbacks', () => {
  for (const { period, timezone, now, expected } of fixture.calendarRange) {
    expect(calendarRange(period, timezone, new Date(now)), `${period} ${timezone} ${now}`).toEqual(
      expected,
    )
  }
})

it('moves whole days, weeks, months and years without overflow', () => {
  for (const { period, from, direction, expected } of fixture.shiftCalendarRange) {
    expect(shiftCalendarRange(period, from, direction), `${period} ${from} ${direction}`).toEqual(
      expected,
    )
  }
})

it('labels the visible calendar range without using the browser timezone', () => {
  for (const { period, from, to, expected } of fixture.calendarRangeLabel) {
    expect(calendarRangeLabel(period, from, to)).toBe(expected)
  }
})
