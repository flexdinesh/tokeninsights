import { expect, it } from 'vitest'
import { formatCoverageTime } from './SyncCoverage'

it('formats server offset labels without treating them as IANA timezone identifiers', () => {
  const timestamp = Date.parse('2026-09-26T00:30:00Z')
  const options: Intl.DateTimeFormatOptions = {
    hour: '2-digit',
    minute: '2-digit',
    timeZone: 'UTC',
  }
  expect(formatCoverageTime(timestamp, 'AEST +10:00')).toBe(
    new Date('2026-09-26T10:30:00Z').toLocaleTimeString([], options),
  )
  expect(formatCoverageTime(timestamp, 'NDT -02:30')).toBe(
    new Date('2026-09-25T22:00:00Z').toLocaleTimeString([], options),
  )
  expect(formatCoverageTime(timestamp, 'Australia/Sydney')).toBe(
    new Date('2026-09-26T10:30:00Z').toLocaleTimeString([], options),
  )
  expect(formatCoverageTime(timestamp, 'unrecognized')).toContain('UTC')
})
