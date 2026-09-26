import { afterEach, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CoverageIndicator, formatCoverageTime, SyncCoverage } from './SyncCoverage'
import type { CoverageDay } from './SyncCoverage'

afterEach(cleanup)

const day: CoverageDay = {
  day: '2026-09-26',
  status: 'checked',
  checkedAt: Date.parse('2026-09-26T00:30:00Z'),
  pendingSources: 0,
  failedSources: 0,
  hasUsage: true,
  total: 42,
}

it.each([
  { status: 'checked', label: `Checked ${formatCoverageTime(day.checkedAt, 'UTC')}` },
  { status: 'empty', label: 'No usage found' },
  { status: 'pending', label: 'Pending · 2 sources', pendingSources: 2 },
  { status: 'partial', label: 'Partial · updating' },
  { status: 'partial', label: 'Incomplete · 1 failed', failedSources: 1 },
  { status: 'unverified', label: 'Not checked' },
] satisfies {
  status: CoverageDay['status']
  label: string
  pendingSources?: number
  failedSources?: number
}[])('keeps $label accessible on compact markers', (example) => {
  render(<CoverageIndicator day={{ ...day, ...example }} timezone="UTC" checkedSince={0} />)
  expect(screen.getByRole('img', { name: example.label })).toHaveAttribute('title', example.label)
  expect(screen.queryByText(example.label)).not.toBeInTheDocument()
})

it('keeps diagnostic counts and day details collapsed until requested', async () => {
  const user = userEvent.setup()
  render(<SyncCoverage days={[day]} timezone="UTC" checkedSince={0} />)
  expect(screen.getByText('Source coverage')).toBeVisible()
  expect(screen.getByText(/1 \/ 1 days checked/)).not.toBeVisible()
  expect(
    screen.getByRole('region', { name: 'Daily source coverage', hidden: true }),
  ).not.toBeVisible()
  await user.click(screen.getByText('Source coverage'))
  expect(screen.getByText(/1 \/ 1 days checked/)).toBeVisible()
  expect(screen.getByRole('region', { name: 'Daily source coverage' })).toBeVisible()
  expect(screen.getByText(day.day)).toBeVisible()
  expect(screen.getByText('42')).toBeVisible()
})

it.each(['checked', 'empty'] satisfies CoverageDay['status'][])(
  'waits for the current check before showing a saved %s marker',
  (status) => {
    const saved = {
      ...day,
      status,
      hasUsage: status !== 'empty',
      total: status === 'empty' ? 0 : day.total,
    }
    const startedAt = day.checkedAt + 60_000
    const { rerender } = render(<CoverageIndicator day={saved} timezone="UTC" />)
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    rerender(<CoverageIndicator day={saved} timezone="UTC" checkedSince={startedAt} />)
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    rerender(
      <CoverageIndicator
        day={{
          ...saved,
          status: 'partial',
          checkedAt: 0,
          pendingSources: 1,
          hasUsage: true,
          total: 42,
        }}
        timezone="UTC"
        checkedSince={startedAt}
      />,
    )
    expect(screen.getByRole('img', { name: 'Partial · updating' })).toBeVisible()
    rerender(
      <CoverageIndicator
        day={{ ...saved, checkedAt: startedAt }}
        timezone="UTC"
        checkedSince={startedAt}
      />,
    )
    expect(screen.getByRole('img')).toBeVisible()
  },
)

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
