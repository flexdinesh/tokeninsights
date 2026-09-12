import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import type { Row, Sort } from '../contracts'
import { exactCount, formatCount } from '../format'
import { useDashboardState } from '../state'
import { BucketControl } from './Filters'
import { Button } from './ui/button'
import { Card } from './ui/card'

const metrics: { key: Sort; label: string }[] = [
  { key: 'total', label: 'Total' },
  { key: 'input', label: 'Input' },
  { key: 'output', label: 'Output' },
  { key: 'cacheRead', label: 'Cache read' },
  { key: 'cacheWrite', label: 'Cache write' },
  { key: 'reasoning', label: 'Reasoning' },
]

const axisTick = { fill: 'var(--color-text-muted)', fontSize: 'var(--text-xs)' }
const tooltipStyle = {
  background: 'var(--color-surface)',
  color: 'var(--color-text-primary)',
  border: 'var(--border-width) solid var(--color-border)',
  borderRadius: 'var(--radius-md)',
  padding: 'var(--space-3)',
  boxShadow: 'var(--shadow-overlay)',
  fontSize: 'var(--text-xs)',
}

function tooltipFormatter(value: unknown): string {
  return typeof value === 'number' ? exactCount(value) : String(value)
}

function legendFormatter(value: string | number) {
  return <span className="chart-legend-label">{value}</span>
}

export function UsageChart({ rows }: { rows: Row[] }) {
  const {
    state: { query, chartMetric: metric },
    dispatch,
  } = useDashboardState()
  const timeline = query.tab === 'tokens' || query.tab === 'sessions'
  const context = query.tab === 'context'
  const title = timeline
    ? 'Usage over time'
    : context
      ? 'Session peak context'
      : `Usage by ${query.tab.slice(0, -1)}`
  const filterDimension =
    query.tab === 'models'
      ? 'models'
      : query.tab === 'providers'
        ? 'providers'
        : query.tab === 'harnesses'
          ? 'harnesses'
          : undefined
  const chartRows = rows.map((r) => ({
    ...r,
    label: context ? `${r.model} · ${r.harness} · ${r.provider}` : r.name,
  }))
  return (
    <Card className="chart-panel panel" role="region" aria-label={title}>
      <div className="panel-heading">
        <div>
          <h2>{title}</h2>
          <p>
            {timeline
              ? 'Token usage across the selected range'
              : context
                ? 'Top 12 groups by average in-range session peak'
                : 'Top 12 by total tokens · select a label to filter'}
          </p>
        </div>
        {timeline ? (
          <BucketControl />
        ) : (
          <span className="eyebrow">{context ? 'Prompt-side tokens' : 'Countable tokens'}</span>
        )}
      </div>
      {timeline && (
        <div className="chart-metrics" aria-label="Chart metric">
          {metrics.map((m) => (
            <Button
              key={m.key}
              variant="ghost"
              size="sm"
              aria-pressed={metric === m.key}
              onClick={() => dispatch({ type: 'chartMetric', value: m.key })}
            >
              {m.label}
            </Button>
          ))}
        </div>
      )}
      {rows.length === 0 ? (
        <div className="chart-empty">No usage in this range</div>
      ) : (
        <div className="chart-canvas">
          <ResponsiveContainer width="100%" height="100%">
            {timeline ? (
              <AreaChart
                data={chartRows}
                margin={{ top: 12, right: 12, left: 0, bottom: 0 }}
                accessibilityLayer
              >
                <defs>
                  <linearGradient id="usage-fill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="var(--chart-1)" stopOpacity={0.24} />
                    <stop offset="100%" stopColor="var(--chart-1)" stopOpacity={0.015} />
                  </linearGradient>
                </defs>
                <CartesianGrid
                  vertical={false}
                  stroke="var(--color-border)"
                  strokeDasharray="3 5"
                />
                <XAxis
                  dataKey="label"
                  tickLine={false}
                  axisLine={false}
                  minTickGap={40}
                  tick={axisTick}
                />
                <YAxis
                  tickFormatter={formatCount}
                  tickLine={false}
                  axisLine={false}
                  width={60}
                  tick={axisTick}
                />
                <Tooltip
                  formatter={tooltipFormatter}
                  contentStyle={tooltipStyle}
                  itemStyle={{ color: 'var(--color-text-primary)' }}
                />
                <Area
                  type="linear"
                  dataKey={metric}
                  name={metrics.find((m) => m.key === metric)?.label}
                  stroke="var(--chart-1)"
                  strokeWidth={2}
                  fill="url(#usage-fill)"
                  isAnimationActive={false}
                  dot={rows.length === 1}
                />
              </AreaChart>
            ) : (
              <BarChart
                data={chartRows}
                maxBarSize={64}
                margin={{ top: 12, right: 12, left: 0, bottom: 0 }}
                accessibilityLayer
              >
                <CartesianGrid
                  vertical={false}
                  stroke="var(--color-border)"
                  strokeDasharray="3 5"
                />
                <XAxis
                  dataKey="label"
                  tickLine={false}
                  axisLine={false}
                  tickFormatter={(v) =>
                    typeof v === 'string' && v.length > 17 ? `${v.slice(0, 15)}…` : String(v)
                  }
                  tick={axisTick}
                />
                <YAxis
                  tickFormatter={formatCount}
                  tickLine={false}
                  axisLine={false}
                  width={60}
                  tick={axisTick}
                />
                <Tooltip
                  formatter={tooltipFormatter}
                  contentStyle={tooltipStyle}
                  itemStyle={{ color: 'var(--color-text-primary)' }}
                />
                {context ? (
                  <>
                    <Legend formatter={legendFormatter} />
                    <Bar
                      dataKey="averageContext"
                      name="Average"
                      fill="var(--chart-1)"
                      isAnimationActive={false}
                    />
                    <Bar
                      dataKey="medianContext"
                      name="Median"
                      fill="var(--color-chart-secondary)"
                      isAnimationActive={false}
                    />
                    <Bar
                      dataKey="maxContext"
                      name="Maximum"
                      fill="var(--color-chart-tertiary)"
                      isAnimationActive={false}
                    />
                  </>
                ) : (
                  <Bar
                    dataKey="total"
                    name="Total tokens"
                    fill="var(--chart-1)"
                    radius={[4, 4, 0, 0]}
                    isAnimationActive={false}
                  />
                )}
              </BarChart>
            )}
          </ResponsiveContainer>
        </div>
      )}
      {filterDimension && rows.length > 0 && (
        <div className="chart-drilldowns" aria-label="Filter by chart item">
          {rows.map((row) => (
            <Button
              key={row.key}
              variant="ghost"
              size="sm"
              className="chart-filter"
              onClick={() =>
                dispatch({ type: 'selection', value: { [filterDimension]: [row.name] } })
              }
              title={`Filter ${row.name}`}
            >
              <span className="legend-dot" />
              {row.name}
            </Button>
          ))}
        </div>
      )}
      {context && (
        <p className="chart-note">
          Context = input + cache read + cache write. Each session contributes its peak within the
          selected range.
        </p>
      )}
    </Card>
  )
}
