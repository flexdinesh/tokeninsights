import type { Dashboard } from '../contracts'
import { exactCount, formatCount } from '../format'
import { Readout, ReadoutStrip } from '@/ui'

export function SummaryCards({
  summary,
  estimated = false,
}: {
  summary: Dashboard['summary']
  estimated?: boolean
}) {
  const readouts: { label: string; value: number; detail: string }[] = [
    {
      label: estimated ? 'Excluded tokens' : 'Total tokens',
      value: summary.total,
      detail: estimated ? 'Excluded from your usage total' : 'Included in your usage total',
    },
    { label: 'Input', value: summary.input, detail: 'Uncached prompt tokens' },
    {
      label: 'Output',
      value: summary.output,
      detail: `${formatCount(summary.reasoning)} reasoning tokens reported`,
    },
    {
      label: 'Cache read',
      value: summary.cacheRead,
      detail: `${formatCount(summary.cacheWrite)} cache write tokens`,
    },
    {
      label: estimated ? 'Excluded sessions shown' : 'Sessions shown',
      value: summary.sessions,
      detail: `${exactCount(summary.syncedSessions)} ${estimated ? 'with excluded usage' : 'synced'} across all dates & harnesses`,
    },
  ]
  return (
    <ReadoutStrip
      aria-label={estimated ? 'Filtered excluded usage summary' : 'Filtered usage summary'}
    >
      {readouts.map((readout, index) => (
        <Readout
          key={readout.label}
          primary={index === 0}
          label={readout.label}
          value={formatCount(readout.value)}
          valueLabel={`${readout.label}: ${exactCount(readout.value)}`}
          detail={readout.detail}
        />
      ))}
    </ReadoutStrip>
  )
}
