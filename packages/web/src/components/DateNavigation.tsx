import { ChevronLeft, ChevronRight } from 'lucide-react'
import { calendarRange, calendarRangeLabel, shiftCalendarRange } from '../dateRange'
import { useDashboardQuery } from '../state'
import { Button } from '@/ui'

export function DateNavigation({ timezone }: { timezone: string }) {
  const { query, updateSelection } = useDashboardQuery()
  const custom = Boolean((query.from || query.to) && query.dateMode !== 'calendar')
  if (custom || query.period === 'all') return null

  const period = query.period
  const current = calendarRange(period === 'yesterday' ? 'today' : period, timezone)
  const range =
    query.from && query.to ? { from: query.from, to: query.to } : calendarRange(period, timezone)
  const unit = period === 'today' || period === 'yesterday' ? 'day' : period
  const atCurrent = range.from >= current.from
  function move(direction: -1 | 1) {
    const next = shiftCalendarRange(period, range.from, direction)
    updateSelection({
      period: period === 'yesterday' ? 'today' : period,
      from: next.from === current.from ? '' : next.from,
      to: next.from === current.from ? '' : next.to,
      dateMode: next.from === current.from ? undefined : 'calendar',
    })
  }

  return (
    <nav className="date-navigation" aria-label="Period navigation">
      <Button
        variant="outline"
        size="icon-compact"
        aria-label={`Previous ${unit}`}
        title={`Previous ${unit}`}
        onClick={() => move(-1)}
      >
        <ChevronLeft size="1em" />
      </Button>
      <span className="date-navigation-label" aria-live="polite" aria-atomic="true">
        {calendarRangeLabel(period, range.from, range.to)}
      </span>
      <Button
        variant="outline"
        size="icon-compact"
        aria-label={`Next ${unit}`}
        title={`Next ${unit}`}
        disabled={atCurrent}
        onClick={() => move(1)}
      >
        <ChevronRight size="1em" />
      </Button>
    </nav>
  )
}
