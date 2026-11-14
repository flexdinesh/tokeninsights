'use client'

import { Stage } from '../../stageshow/stage'
import type { Act } from '../../stageshow/act'
import { Window } from '../../stageshow/window'
import { Count } from '../../stageshow/motion/Count'
import { AppSidebar, Connection, Readout, RefreshButton } from './app'
import { UsageChart } from './UsageChart'
import './hero.css'

// The landing hero: the desktop app on its Tokens view, with demo data. The
// pointer presses Refresh, capture runs, and the day's new usage lands: the
// readouts roll, today's point on the chart rises and today's row updates.
// It plays once when it comes into view and holds the refreshed dashboard.

type S = { pending: boolean; captured: boolean; totals: boolean }

const initial: S = { pending: false, captured: false, totals: false }
const POSTER: S = { pending: false, captured: true, totals: true }

const WIDE = { width: 1198, height: 500 }
const COMPACT = { below: 760, width: 358, height: 340 }
const REST = { wide: { x: 1010, y: 116 }, compact: { x: 300, y: 130 } }

// 1 to 14 September; capture adds 24K to the 14th
const DAYS_K = [31, 44, 38, 52, 29, 18, 47, 63, 71, 58, 40, 22, 66, 68]
const TODAY_AFTER_K = 92
const GRID_K = [0, 25, 50, 75, 100]

type Figure = { label: string; before: number; after: number; detail: string; unit: string }
const READOUTS: Figure[] = [
  {
    label: 'Total tokens',
    before: 647,
    after: 671,
    detail: 'Included in your usage total',
    unit: 'K',
  },
  { label: 'Input', before: 369, after: 383, detail: 'Uncached prompt tokens', unit: 'K' },
  { label: 'Output', before: 78, after: 83, detail: '13K reasoning tokens reported', unit: 'K' },
  { label: 'Cache read', before: 178, after: 183, detail: '9K cache write tokens', unit: 'K' },
  { label: 'Sessions shown', before: 38, after: 39, detail: 'Across all harnesses', unit: '' },
]
const FILTERS = [
  '2026-09-01 to 09-14',
  'Harness: All',
  'Provider: All',
  'Model: All',
  'Session: All',
]
const METRICS = ['Total', 'Input', 'Output', 'Cache read', 'Cache write', 'Reasoning']
const COLUMNS = ['Sessions', 'Total', 'Input', 'Output', 'Cache R', 'Cache W', 'Reasoning']
const TODAY = {
  period: '2026-09-14',
  before: [4, 68, 39, 9, 18, 1, 1],
  after: [5, 92, 53, 14, 23, 1, 1],
}
const EARLIER = [
  { period: '2026-09-13', cells: ['5', '66K', '38K', '8K', '18K', '1K', '1K'] },
  { period: '2026-09-12', cells: ['3', '22K', '13K', '2K', '6K', '0.4K', '0.6K'] },
]
const COMPACT_COLUMNS = 2
const COMPACT_READOUTS = 2
// Two turns of the Refresh icon (app.css, ti-app-spin)
const CAPTURE_MS = 1400

const withUnit = (unit: string) => (n: number) => `${Math.round(n)}${unit}`
const dayLabel = (day: number) => `09-${String(day + 1).padStart(2, '0')}`

async function script(a: Act<S>) {
  await a.wait(900)
  await a.move({ at: 'refresh', x: 0.16, y: 0.72 })
  await a.wait(250)
  await a.click()
  await a.set({ pending: true })
  // Off the button while capture runs, so the pointer never sits on the
  // label as it changes. Capture lasts two whole turns of the icon, so it
  // stops where it started.
  void a.move(REST[a.layout])
  await a.wait(CAPTURE_MS)
  await a.set({ pending: false, captured: true })
  await a.wait(160)
  await a.set({ totals: true })
  await a.wait(2600)
}

function Scene({ s, compact }: { s: S; compact: boolean }) {
  const days = s.captured ? [...DAYS_K.slice(0, -1), TODAY_AFTER_K] : DAYS_K
  const reads = compact ? READOUTS.slice(0, COMPACT_READOUTS) : READOUTS
  const columns = compact ? COLUMNS.slice(0, COMPACT_COLUMNS) : COLUMNS
  const today = s.captured ? TODAY.after : TODAY.before
  const lead = (
    <span className="ti-app-title">
      <span>/ </span>Tokens
      {!compact && <span className="ti-app-tag">Demo data</span>}
    </span>
  )
  const toolbar = (
    <>
      {!compact && (
        <>
          <Connection hostname="laptop" line="LOCAL · local · connected" />
          <span className="ti-app-rule" />
        </>
      )}
      <RefreshButton pending={s.pending} target="refresh" />
    </>
  )
  const body = (
    <>
      {!compact && (
        <div className="ti-hero-filters">
          <span>Filters</span>
          {FILTERS.map((filter) => (
            <span key={filter} className="ti-hero-ctl">
              {filter}
            </span>
          ))}
        </div>
      )}
      <div className="ti-app-reads ti-hero-reads">
        {reads.map((figure) => (
          <Readout
            key={figure.label}
            label={figure.label}
            value={
              <Count
                value={s.totals ? figure.after : figure.before}
                format={withUnit(figure.unit)}
              />
            }
            detail={compact ? undefined : figure.detail}
          />
        ))}
      </div>
      <div className="ti-app-sec">
        <div className="ti-app-sh">
          <span>Usage over time</span>
          <span className="ti-app-seg">
            {(compact ? METRICS.slice(0, 3) : METRICS).map((metric) => (
              <span key={metric} data-on={metric === 'Total' || undefined}>
                {metric}
              </span>
            ))}
          </span>
        </div>
        <UsageChart
          width={compact ? 298 : 930}
          height={compact ? 130 : 220}
          maxK={100}
          gridK={GRID_K}
          daysK={days}
          dayLabel={dayLabel}
        />
      </div>
      <div className="ti-app-sec ti-app-table">
        <div className="ti-app-sh">
          <span>Tokens breakdown</span>
        </div>
        <table>
          <thead>
            <tr>
              <th>Period</th>
              {columns.map((column) => (
                <th key={column}>{column}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>{TODAY.period}</td>
              {columns.map((column, index) => (
                <td key={column}>
                  <Count value={today[index] ?? 0} format={withUnit(index === 0 ? '' : 'K')} />
                </td>
              ))}
            </tr>
            {EARLIER.map((row) => (
              <tr key={row.period}>
                <td>{row.period}</td>
                {columns.map((column, index) => (
                  <td key={column}>{row.cells[index]}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  )
  return compact ? (
    <Window className="ti-hero-window" lights="grey" lead={lead} toolbar={toolbar}>
      {body}
    </Window>
  ) : (
    <Window
      className="ti-hero-window"
      lights="grey"
      sidebar={<AppSidebar note />}
      sidebarWidth={176}
      lead={lead}
      toolbar={toolbar}
    >
      {body}
    </Window>
  )
}

export function HeroDemo() {
  return (
    <Stage
      width={WIDE.width}
      height={WIDE.height}
      compact={COMPACT}
      play="once"
      initial={initial}
      poster={POSTER}
      script={script}
      cursor={{ at: REST }}
      fidelity="realistic"
      className="ti-demo ti-hero-demo"
    >
      {(s, _rect, layout) => <Scene s={s} compact={layout === 'compact'} />}
    </Stage>
  )
}
