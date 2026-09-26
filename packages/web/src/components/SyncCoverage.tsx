import type { Dashboard } from '../contracts'
import { formatCount } from '../format'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from './ui/table'

export type CoverageDay = NonNullable<Dashboard['coverage']>[number]

const minutesPerHour = 60
const millisecondsPerMinute = 60_000

export function formatCoverageTime(timestamp: number, timezone?: string): string {
  const options: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit' }
  const offset = timezone?.match(/([+-])(\d{2}):(\d{2})$/)
  if (offset) {
    const minutes = Number(offset[2]) * minutesPerHour + Number(offset[3])
    const direction = offset[1] === '-' ? -1 : 1
    return new Date(timestamp + direction * minutes * millisecondsPerMinute).toLocaleTimeString(
      [],
      { ...options, timeZone: 'UTC' },
    )
  }
  try {
    return new Date(timestamp).toLocaleTimeString([], { ...options, timeZone: timezone })
  } catch {
    return `${new Date(timestamp).toLocaleTimeString([], { ...options, timeZone: 'UTC' })} UTC`
  }
}

export function coverageLabel(day: CoverageDay, timezone?: string): string {
  switch (day.status) {
    case 'checked':
      return `Checked ${formatCoverageTime(day.checkedAt, timezone)}`
    case 'empty':
      return 'No usage found'
    case 'partial':
      return day.failedSources > 0
        ? `Incomplete · ${day.failedSources} failed`
        : 'Partial · updating'
    case 'pending':
      return `Pending · ${day.pendingSources} sources`
    case 'unverified':
    default:
      return 'Not checked'
  }
}

export function SyncCoverage({ days, timezone }: { days: CoverageDay[]; timezone?: string }) {
  if (days.length === 0) return null
  const checked = days.filter((day) => day.status === 'checked' || day.status === 'empty').length
  const newest = days.at(-1)
  const newestFirst: CoverageDay[] = []
  for (let index = days.length - 1; index >= 0; index--) {
    const day = days[index]
    if (day) newestFirst.push(day)
  }
  return (
    <details className="sync-coverage">
      <summary>
        <span>Source coverage</span>
        <span>
          {checked} / {days.length} days checked
        </span>
        {newest && (
          <span className="coverage-latest">
            {newest.day} · {coverageLabel(newest, timezone)}
          </span>
        )}
      </summary>
      <p>
        Retained local sources, as checked. Pending or incomplete days may gain usage. Dates and
        check times use {timezone ?? 'the source server’s timezone'}.
      </p>
      <div
        className="coverage-scroll"
        tabIndex={0}
        role="region"
        aria-label="Daily source coverage"
      >
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Day</TableHead>
              <TableHead className="numeric">Tokens found</TableHead>
              <TableHead>Source status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {newestFirst.map((day) => (
              <TableRow key={day.day}>
                <TableCell>{day.day}</TableCell>
                <TableCell className="numeric">
                  {day.total === null ? '—' : formatCount(day.total)}
                </TableCell>
                <TableCell data-status={day.status}>
                  {coverageLabel(day, timezone)}
                  {day.status === 'empty' && (
                    <small> · Checked {formatCoverageTime(day.checkedAt, timezone)}</small>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </details>
  )
}
