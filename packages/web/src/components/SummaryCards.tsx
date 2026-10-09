import type { Dashboard } from '../contracts'
import { exactCount, formatCount } from '../format'

export function SummaryCards({
  summary,
  estimated = false,
}: {
  summary: Dashboard['summary']
  estimated?: boolean
}) {
  const cards: {
    label: string
    value: number
    detail: string
    className: string
  }[] = [
    {
      label: estimated ? 'Excluded tokens' : 'Total tokens',
      value: summary.total,
      detail: estimated ? 'Excluded from your usage total' : 'Included in your usage total',
      className: 'total-card',
    },
    {
      label: 'Input',
      value: summary.input,
      detail: 'Uncached prompt tokens',
      className: 'input-card',
    },
    {
      label: 'Output',
      value: summary.output,
      detail: `${formatCount(summary.reasoning)} reasoning tokens reported`,
      className: 'output-card',
    },
    {
      label: 'Cache read',
      value: summary.cacheRead,
      detail: `${formatCount(summary.cacheWrite)} cache write tokens`,
      className: 'cache-card',
    },
    {
      label: estimated ? 'Excluded sessions shown' : 'Sessions shown',
      value: summary.sessions,
      detail: `${exactCount(summary.syncedSessions)} ${estimated ? 'with excluded usage' : 'synced'} across all dates & harnesses`,
      className: 'sessions-card',
    },
  ]
  return (
    <section
      className="summary-grid"
      aria-label={estimated ? 'Filtered excluded usage summary' : 'Filtered usage summary'}
    >
      {cards.map((card) => (
        <div key={card.label} className={`summary-card ${card.className}`}>
          <span className="metric-label">{card.label}</span>
          <strong aria-label={`${card.label}: ${exactCount(card.value)}`}>
            {formatCount(card.value)}
          </strong>
          <span
            className={`card-detail${estimated && card.className === 'total-card' ? ' essential-detail' : ''}`}
          >
            {card.detail}
          </span>
        </div>
      ))}
    </section>
  )
}
