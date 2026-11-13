import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { expect, it } from 'vitest'
import { z } from 'zod'
import { tabSchema } from '@tokeninsights/contracts'
import {
  exactCount,
  formatCount,
  formatServerDate,
  formatServerDateTime,
  labels,
  serverTimeZoneLabel,
} from '../../dashboard/src/format'

// Shared with packages/core/dashboardview so native formatting stays identical.
const countCases = z.array(z.strictObject({ value: z.number(), expected: z.string() }))
const instantCases = z.array(
  z.strictObject({ instant: z.iso.datetime(), timezone: z.string(), expected: z.string() }),
)
const fixture = z
  .strictObject({
    compactCount: countCases,
    exactCount: countCases,
    tokenShare: z.array(
      z.strictObject({ tokens: z.number(), total: z.number(), expected: z.string() }),
    ),
    serverZoneLabel: z.array(z.strictObject({ timezone: z.string(), expected: z.string() })),
    serverDate: instantCases,
    serverDateTime: instantCases,
    tabLabels: z.record(tabSchema, z.string()),
  })
  .parse(
    JSON.parse(
      readFileSync(
        join(import.meta.dirname, '../../contracts/fixtures/dashboard-view/format.json'),
        'utf8',
      ),
    ),
  )

it('formats compact and exact counts', () => {
  for (const { value, expected } of fixture.compactCount) expect(formatCount(value)).toBe(expected)
  for (const { value, expected } of fixture.exactCount) expect(exactCount(value)).toBe(expected)
})

it('labels server zones and falls back explicitly to UTC', () => {
  for (const { timezone, expected } of fixture.serverZoneLabel) {
    expect(serverTimeZoneLabel(timezone)).toBe(expected)
  }
})

it('formats server dates on the server calendar with historical DST offsets', () => {
  for (const { instant, timezone, expected } of fixture.serverDate) {
    expect(formatServerDate(Date.parse(instant), timezone)).toBe(expected)
  }
  for (const { instant, timezone, expected } of fixture.serverDateTime) {
    expect(formatServerDateTime(Date.parse(instant), timezone)).toBe(expected)
  }
})

it('labels every tab', () => {
  expect(labels).toEqual(fixture.tabLabels)
})
