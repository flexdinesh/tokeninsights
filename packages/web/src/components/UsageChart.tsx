import { Area, AreaChart, Bar, BarChart, CartesianGrid, Legend, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { Row, Sort } from '../contracts'
import { exactCount, formatCount } from '../format'
import { useDashboardState } from '../state'
import { BucketControl } from './Filters'

const metrics: { key: Sort; label: string }[] = [{ key: 'total', label: 'Total' }, { key: 'input', label: 'Input' }, { key: 'output', label: 'Output' }, { key: 'cacheRead', label: 'Cache read' }, { key: 'cacheWrite', label: 'Cache write' }, { key: 'reasoning', label: 'Reasoning' }]

export function UsageChart({ rows }: { rows: Row[] }) {
  const { state: { query, chartMetric: metric }, dispatch } = useDashboardState()
  const timeline = query.tab === 'tokens' || query.tab === 'sessions'
  const context = query.tab === 'context'
  const title = timeline ? 'Usage over time' : context ? 'Session peak context' : `Usage by ${query.tab.slice(0, -1)}`
  const filterDimension = query.tab === 'models' ? 'models' : query.tab === 'providers' ? 'providers' : query.tab === 'harnesses' ? 'harnesses' : undefined
  const tooltipFormatter = (value: unknown) => typeof value === 'number' ? exactCount(value) : String(value)
  const chartRows = rows.map(r => ({ ...r, label: context ? `${r.model} · ${r.harness} · ${r.provider}` : r.name }))
  return <section className="chart-panel panel" aria-label={title}>
    <div className="panel-heading"><div><h2>{title}</h2><p>{timeline ? 'Token usage across the selected range' : context ? 'Top 12 groups by average in-range session peak' : 'Top 12 by total tokens · select a label to filter'}</p></div>
      {timeline ? <BucketControl /> : <span className="eyebrow">{context ? 'Prompt-side tokens' : 'Countable tokens'}</span>}
    </div>
    {timeline && <div className="chart-metrics" aria-label="Chart metric">{metrics.map(m => <button key={m.key} aria-pressed={metric === m.key} onClick={() => dispatch({ type: 'chartMetric', value: m.key })}>{m.label}</button>)}</div>}
    {rows.length === 0 ? <div className="chart-empty">No usage in this range</div> : <div className="chart-canvas">
      <ResponsiveContainer width="100%" height="100%">
        {timeline ? <AreaChart data={chartRows} margin={{ top: 12, right: 12, left: 0, bottom: 0 }} accessibilityLayer>
          <defs><linearGradient id="usage-fill" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stopColor="var(--accent)" stopOpacity={0.24} /><stop offset="100%" stopColor="var(--accent)" stopOpacity={0.015} /></linearGradient></defs>
          <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="3 5" />
          <XAxis dataKey="label" tickLine={false} axisLine={false} minTickGap={40} tick={{ fill: 'var(--muted)', fontSize: '0.75rem' }} />
          <YAxis tickFormatter={formatCount} tickLine={false} axisLine={false} width={60} tick={{ fill: 'var(--muted)', fontSize: '0.75rem' }} />
          <Tooltip formatter={tooltipFormatter} contentStyle={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: '0.6rem', color: 'var(--text)' }} />
          <Area type="linear" dataKey={metric} name={metrics.find(m => m.key === metric)?.label} stroke="var(--accent)" strokeWidth={2} fill="url(#usage-fill)" isAnimationActive={false} dot={rows.length === 1} />
        </AreaChart> : <BarChart data={chartRows} margin={{ top: 12, right: 12, left: 0, bottom: 0 }} accessibilityLayer>
          <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="3 5" />
          <XAxis dataKey="label" tickLine={false} axisLine={false} tickFormatter={v => typeof v === 'string' && v.length > 17 ? `${v.slice(0, 15)}…` : String(v)} tick={{ fill: 'var(--muted)', fontSize: '0.7rem' }} />
          <YAxis tickFormatter={formatCount} tickLine={false} axisLine={false} width={60} tick={{ fill: 'var(--muted)', fontSize: '0.75rem' }} />
          <Tooltip formatter={tooltipFormatter} contentStyle={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: '0.6rem', color: 'var(--text)' }} />
          {context ? <><Legend /><Bar dataKey="averageContext" name="Average" fill="var(--accent)" isAnimationActive={false} /><Bar dataKey="medianContext" name="Median" fill="var(--chart-blue)" isAnimationActive={false} /><Bar dataKey="maxContext" name="Maximum" fill="var(--chart-purple)" isAnimationActive={false} /></> : <Bar dataKey="total" name="Total tokens" fill="var(--accent)" radius={[4, 4, 0, 0]} maxBarSize={64} isAnimationActive={false} />}
        </BarChart>}
      </ResponsiveContainer>
    </div>}
    {filterDimension && rows.length > 0 && <div className="chart-drilldowns" aria-label="Filter by chart item">{rows.map(row => <button key={row.key} className="chart-filter" onClick={() => dispatch({ type: 'selection', value: { [filterDimension]: [row.name] } })} title={`Filter ${row.name}`}><span className="legend-dot" />{row.name}</button>)}</div>}
    {context && <p className="chart-note">Context = input + cache read + cache write. Each session contributes its peak within the selected range.</p>}
  </section>
}
