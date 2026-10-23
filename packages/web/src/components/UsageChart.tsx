import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  LabelList,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import type { Row, Sort } from '../contracts'
import { exactCount, formatCount, formatServerDateTime, serverTimeZoneLabel } from '../format'
import { useDashboardPreferences, useDashboardQuery } from '../state'
import { BucketControl } from './Filters'
import { Button, cn, SectionHeader, SegmentedControl } from '@/ui'

const metrics: { key: Sort; label: string }[] = [
  { key: 'total', label: 'Total' },
  { key: 'input', label: 'Input' },
  { key: 'output', label: 'Output' },
  { key: 'cacheRead', label: 'Cache read' },
  { key: 'cacheWrite', label: 'Cache write' },
  { key: 'reasoning', label: 'Reasoning' },
]

const categoricalColors = [
  'var(--chart-1)',
  'var(--chart-2)',
  'var(--chart-3)',
  'var(--chart-4)',
  'var(--chart-5)',
  'var(--chart-6)',
  'var(--chart-7)',
  'var(--chart-8)',
  'var(--chart-9)',
  'var(--chart-10)',
  'var(--chart-11)',
  'var(--chart-12)',
]

export function categoricalChartColor(index: number): string {
  return categoricalColors[index % categoricalColors.length] ?? categoricalColors[0] ?? ''
}

const axisTick = { fill: 'var(--ink-subtle)', fontSize: 'var(--text-label)' }

function TimelineTick({
  x,
  y,
  index,
  visibleTicksCount,
  payload,
}: {
  x?: number
  y?: number
  index?: number
  visibleTicksCount?: number
  payload?: { value?: string | number }
}) {
  const anchor =
    (visibleTicksCount ?? 0) <= 1
      ? 'middle'
      : index === 0
        ? 'start'
        : index === (visibleTicksCount ?? 0) - 1
          ? 'end'
          : 'middle'
  return (
    <text x={x} y={y} dy="0.8em" textAnchor={anchor} {...axisTick}>
      {payload?.value}
    </text>
  )
}
const tooltipStyle = {
  background: 'var(--surface-overlay)',
  color: 'var(--ink)',
  border: 'var(--border-width) solid var(--line)',
  borderRadius: 'var(--radius-md)',
  padding: 'var(--space-3)',
  boxShadow: 'var(--shadow-overlay)',
  fontSize: 'var(--text-label)',
}

function tooltipFormatter(value: unknown): string {
  return typeof value === 'number' ? exactCount(value) : String(value)
}

const tokenShareFormat = new Intl.NumberFormat('en', {
  style: 'percent',
  maximumFractionDigits: 1,
})
const minimumDisplayedShare = 0.001

export function formatTokenShare(tokens: number, totalTokens: number): string {
  const share = totalTokens > 0 ? tokens / totalTokens : 0
  return share > 0 && share < minimumDisplayedShare
    ? `<${tokenShareFormat.format(minimumDisplayedShare)}`
    : tokenShareFormat.format(share)
}

// Leave room for a full tick line when users enlarge text to 200%.
const categoryAxisHeight = 48
// Keep percentage labels readable when many categories share a narrow viewport.
const categoryShareWidthRem = 3.5
const categoryChartAxisWidthRem = 4
const contextLegend = [
  { label: 'Average', color: 'var(--chart-1)' },
  { label: 'Median', color: 'var(--chart-2)' },
  { label: 'Maximum', color: 'var(--chart-3)' },
]

export function UsageChart({
  rows,
  totalTokens,
  timezone,
}: {
  rows: Row[]
  totalTokens: number
  timezone: string
}) {
  const { query, updateSelection } = useDashboardQuery()
  const { chartMetric: metric, setChartMetric } = useDashboardPreferences()
  const timeline = query.tab === 'tokens' || query.tab === 'sessions'
  const context = query.tab === 'context'
  const baseTitle = timeline
    ? 'Usage over time'
    : context
      ? 'Session peak context'
      : query.tab === 'repo'
        ? `Usage by ${query.locationGroup}`
        : query.tab === 'harnesses'
          ? 'Usage by harness'
          : `Usage by ${query.tab.slice(0, -1)}`
  const reviewingExcluded = query.quality === 'estimated'
  const title = reviewingExcluded
    ? context
      ? 'Excluded session peak context'
      : baseTitle.replace('Usage', 'Excluded usage')
    : baseTitle
  const filterDimension =
    query.tab === 'models'
      ? 'models'
      : query.tab === 'providers'
        ? 'providers'
        : query.tab === 'harnesses'
          ? 'harnesses'
          : undefined
  const showTokenShares = filterDimension !== undefined || query.tab === 'repo'
  const chartRows = rows.map((r, index) => ({
    ...r,
    label: context ? `${r.model} · ${r.harness} · ${r.provider}` : r.name,
    color: categoricalChartColor(index),
    tokenShare: formatTokenShare(r.total, totalTokens),
  }))
  return (
    <section className="my-2" aria-label={title}>
      <SectionHeader
        className="pt-0"
        title={title}
        hideDescription={timeline}
        description={
          timeline
            ? 'Token usage across the selected range'
            : context
              ? 'Top 12 groups by average in-range session peak'
              : query.tab === 'repo'
                ? 'Top 12 location rows by total tokens · % of filtered total'
                : 'Top 12 by total tokens · % of filtered total · select a label to filter'
        }
        actions={
          timeline ? (
            <>
              <SegmentedControl
                label="Chart metric"
                options={metrics.map((m) => ({ value: m.key, label: m.label }))}
                value={metric}
                onValueChange={setChartMetric}
              />
              <BucketControl />
            </>
          ) : (
            <span className="text-label text-ink-subtle">
              {context
                ? 'Prompt-side tokens'
                : reviewingExcluded
                  ? 'Excluded tokens'
                  : 'Countable tokens'}
            </span>
          )
        }
      />
      {rows.length === 0 ? (
        <div className="grid min-h-(--chart-height) place-items-center p-4 text-center text-body text-ink-subtle">
          {reviewingExcluded ? 'No excluded usage in this range' : 'No usage in this range'}
        </div>
      ) : (
        <div
          className={cn(
            'h-(--chart-height) pr-1 tabular-nums',
            showTokenShares && 'overflow-x-auto',
          )}
        >
          <ResponsiveContainer
            width="100%"
            height="100%"
            minWidth={
              showTokenShares
                ? `${rows.length * categoryShareWidthRem + categoryChartAxisWidthRem}rem`
                : undefined
            }
          >
            {timeline ? (
              <AreaChart
                data={chartRows}
                margin={{ top: 12, right: 12, left: 0, bottom: 0 }}
                accessibilityLayer
              >
                <CartesianGrid vertical={false} stroke="var(--line)" />
                <XAxis
                  dataKey="label"
                  tickLine={false}
                  axisLine={false}
                  minTickGap={40}
                  height={categoryAxisHeight}
                  tick={<TimelineTick />}
                />
                <YAxis
                  tickFormatter={formatCount}
                  tickLine={false}
                  axisLine={false}
                  width="auto"
                  tick={axisTick}
                />
                <Tooltip
                  formatter={tooltipFormatter}
                  labelFormatter={(label: unknown) => {
                    const row = chartRows.find((candidate) => candidate.label === label)
                    return row && row.date > 0
                      ? `${row.label} · ${formatServerDateTime(row.date, timezone)} · ${serverTimeZoneLabel(timezone)}`
                      : String(label)
                  }}
                  contentStyle={tooltipStyle}
                  itemStyle={{ color: 'var(--ink)' }}
                />
                <Area
                  type="linear"
                  dataKey={metric}
                  name={metrics.find((m) => m.key === metric)?.label}
                  stroke="var(--chart-1)"
                  strokeWidth={2}
                  fill="none"
                  isAnimationActive={false}
                  dot={rows.length === 1}
                />
              </AreaChart>
            ) : (
              <BarChart
                data={chartRows}
                maxBarSize={64}
                margin={{ top: showTokenShares ? 28 : 12, right: 12, left: 0, bottom: 0 }}
                accessibilityLayer
              >
                <CartesianGrid vertical={false} stroke="var(--line)" />
                <XAxis
                  dataKey="label"
                  height={categoryAxisHeight}
                  minTickGap={40}
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
                  width="auto"
                  tick={axisTick}
                />
                <Tooltip
                  formatter={(value) =>
                    showTokenShares && typeof value === 'number'
                      ? `${exactCount(value)} (${formatTokenShare(value, totalTokens)})`
                      : tooltipFormatter(value)
                  }
                  contentStyle={tooltipStyle}
                  itemStyle={{ color: 'var(--ink)' }}
                />
                {context ? (
                  <>
                    <Bar
                      dataKey="averageContext"
                      name="Average"
                      fill="var(--chart-1)"
                      isAnimationActive={false}
                    />
                    <Bar
                      dataKey="medianContext"
                      name="Median"
                      fill="var(--chart-2)"
                      isAnimationActive={false}
                    />
                    <Bar
                      dataKey="maxContext"
                      name="Maximum"
                      fill="var(--chart-3)"
                      isAnimationActive={false}
                    />
                  </>
                ) : (
                  <Bar
                    dataKey="total"
                    name="Total tokens"
                    radius={[4, 4, 0, 0]}
                    isAnimationActive={false}
                  >
                    {chartRows.map((row) => (
                      <Cell key={row.key} fill={row.color} />
                    ))}
                    {showTokenShares && (
                      <LabelList
                        dataKey="tokenShare"
                        position="top"
                        fill="var(--ink)"
                        fontSize="var(--text-label)"
                      />
                    )}
                  </Bar>
                )}
              </BarChart>
            )}
          </ResponsiveContainer>
        </div>
      )}
      {filterDimension && rows.length > 0 && (
        <div
          className="flex flex-wrap justify-center gap-x-3 gap-y-2 px-(--panel-padding) pb-4"
          role="group"
          aria-label="Filter by chart item"
        >
          {rows.map((row, index) => (
            <Button
              key={row.key}
              variant="link"
              size="compact"
              className="max-w-full justify-start text-left whitespace-normal wrap-anywhere"
              onClick={() => updateSelection({ [filterDimension]: [row.name] })}
              title={`Filter ${row.name}`}
            >
              <LegendSwatch color={categoricalChartColor(index)} />
              <span className="min-w-0">{row.name}</span>
              <span className="shrink-0 whitespace-nowrap tabular-nums">
                {formatTokenShare(row.total, totalTokens)}
              </span>
            </Button>
          ))}
        </div>
      )}
      {context && (
        <ul
          className="flex flex-wrap justify-center gap-x-4 gap-y-2 px-(--panel-padding) pt-2 pb-4 text-label text-ink-muted"
          aria-label="Context chart legend"
        >
          {contextLegend.map(({ label, color }) => (
            <li key={label} className="flex items-center gap-2">
              <LegendSwatch color={color} />
              {label}
            </li>
          ))}
        </ul>
      )}
      {context && (
        <p className="px-(--panel-padding) pb-4 text-label text-ink-subtle">
          Context = input + cache read + cache write. Each session contributes its peak within the
          selected range.
        </p>
      )}
    </section>
  )
}

function LegendSwatch({ color }: { color: string }) {
  return (
    <span
      aria-hidden="true"
      className="inline-block size-2 shrink-0 rounded-sm"
      style={{ backgroundColor: color }}
    />
  )
}
