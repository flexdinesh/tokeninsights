import type { Dashboard } from '../contracts'
import {
  Check,
  ChevronRight,
  CircleAlert,
  CircleHelp,
  CircleMinus,
  Clock3,
  RefreshCw,
} from 'lucide-react'
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

export function coverageConfirmed(day: CoverageDay, checkedSince?: number): boolean {
  return checkedSince !== undefined && day.checkedAt >= checkedSince
}

export function CoverageIndicator({
  day,
  timezone,
  checkedSince,
}: {
  day: CoverageDay
  timezone?: string
  checkedSince?: number
}) {
  if ((day.status === 'checked' || day.status === 'empty') && !coverageConfirmed(day, checkedSince))
    return null
  const label = coverageLabel(day, timezone)
  const Icon =
    day.status === 'checked'
      ? Check
      : day.status === 'empty'
        ? CircleMinus
        : day.status === 'pending'
          ? Clock3
          : day.status === 'partial'
            ? day.failedSources > 0
              ? CircleAlert
              : RefreshCw
            : CircleHelp
  return (
    <span
      className="coverage-indicator"
      data-status={day.status}
      role="img"
      aria-label={label}
      title={label}
    >
      <Icon size="1em" aria-hidden="true" />
    </span>
  )
}

export function SyncCoverage({
  days,
  timezone,
  checkedSince,
}: {
  days: CoverageDay[]
  timezone?: string
  checkedSince?: number
}) {
  if (days.length === 0) return null
  const checked = days.filter(
    (day) =>
      (day.status === 'checked' || day.status === 'empty') && coverageConfirmed(day, checkedSince),
  ).length
  const newestFirst: CoverageDay[] = []
  for (let index = days.length - 1; index >= 0; index--) {
    const day = days[index]
    if (day) newestFirst.push(day)
  }
  return (
    <details className="sync-coverage">
      <summary>
        <ChevronRight size="1em" aria-hidden="true" />
        <span>Source coverage</span>
      </summary>
      <p>
        {checked} / {days.length} days checked. Retained local sources, as checked. Pending or
        incomplete days may gain usage. Dates and check times use{' '}
        {timezone ?? 'the source server’s timezone'}.
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
