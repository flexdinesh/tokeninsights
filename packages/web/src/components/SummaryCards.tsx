import type { Dashboard } from '../contracts'
import { exactCount, formatCount } from '../format'
import { Card } from './ui/card'

export function SummaryCards({ summary }: { summary: Dashboard['summary'] }) {
  const cards = [
    {
      label: 'Total tokens',
      value: summary.total,
      detail: 'Canonical, countable usage',
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
      label: 'Sessions shown',
      value: summary.sessions,
      detail: `${exactCount(summary.syncedSessions)} synced across all dates & harnesses`,
      className: 'sessions-card',
    },
  ]
  return (
    <section className="summary-grid" aria-label="Filtered usage summary">
      {cards.map((card) => (
        <Card key={card.label} className={`summary-card ${card.className}`}>
          <span className="eyebrow">{card.label}</span>
          <strong
            title={exactCount(card.value)}
            aria-label={`${card.label}: ${exactCount(card.value)}`}
          >
            {formatCount(card.value)}
          </strong>
          <span className="card-detail">{card.detail}</span>
        </Card>
      ))}
    </section>
  )
}
