import { Logo } from './Logo'

const VIEWS = ['Tokens', 'Models', 'Providers', 'Harnesses', 'Sessions', 'Context', 'Repo']
const FILTERS = [
  '2026-09-01 to 09-14',
  'Harness: All',
  'Provider: All',
  'Model: All',
  'Session: All',
]
const METRICS = ['Total', 'Input', 'Output', 'Cache read', 'Cache write', 'Reasoning']
const READOUTS = [
  { label: 'Total tokens', value: '647K', detail: 'Included in your usage total' },
  { label: 'Input', value: '369K', detail: 'Uncached prompt tokens' },
  { label: 'Output', value: '78K', detail: '13K reasoning tokens reported' },
  { label: 'Cache read', value: '178K', detail: '9K cache write tokens' },
  { label: 'Sessions shown', value: '38', detail: '41 synced across all dates and harnesses' },
]
const COLUMNS = ['Sessions', 'Total', 'Input', 'Output', 'Cache R', 'Cache W', 'Reasoning']
const ROWS = [
  { period: '2026-09-14', cells: ['4', '68K', '39K', '9K', '18K', '1K', '1K'] },
  { period: '2026-09-13', cells: ['5', '66K', '38K', '8K', '18K', '1K', '1K'] },
  { period: '2026-09-12', cells: ['3', '22K', '13K', '2K', '6K', '0.4K', '0.6K'] },
]
const DAILY_TOTALS_K = [31, 44, 38, 52, 29, 18, 47, 63, 71, 58, 40, 22, 66, 68]

const CHART = { width: 900, height: 190, left: 34, right: 8, top: 8, bottom: 20, maxK: 80 }
const GRID_K = [0, 20, 40, 60, 80]
const LABELLED_DAYS = [0, 3, 6, 9, 13]

const chartX = (day: number) =>
  CHART.left + (day * (CHART.width - CHART.left - CHART.right)) / (DAILY_TOTALS_K.length - 1)
const chartY = (valueK: number) =>
  CHART.top + (CHART.height - CHART.bottom - CHART.top) * (1 - valueK / CHART.maxK)

function anchorFor(day: number) {
  if (day === 0) return 'start'
  if (day === DAILY_TOTALS_K.length - 1) return 'end'
  return 'middle'
}

function UsageChart() {
  const points = DAILY_TOTALS_K.map((value, day) => `${chartX(day)},${chartY(value)}`).join(' ')
  return (
    <svg
      className="ti-chart"
      viewBox={`0 0 ${CHART.width} ${CHART.height}`}
      role="img"
      aria-label="Tokens per day, 1 to 14 September, peaking at 71K on the 9th. Demo data."
    >
      {GRID_K.map((value) => (
        <g key={value}>
          <line
            x1={CHART.left}
            x2={CHART.width - CHART.right}
            y1={chartY(value)}
            y2={chartY(value)}
          />
          <text x={CHART.left - 6} y={chartY(value) + 3.5} textAnchor="end">
            {value === 0 ? '0' : `${value}K`}
          </text>
        </g>
      ))}
      {LABELLED_DAYS.map((day) => (
        <text key={day} x={chartX(day)} y={CHART.height - 5} textAnchor={anchorFor(day)}>
          {`09-${String(day + 1).padStart(2, '0')}`}
        </text>
      ))}
      <polyline points={points} />
    </svg>
  )
}

export function DashboardMock() {
  return (
    <div className="ti-dash">
      <aside>
        <Logo />
        <div className="ti-dash-nav">
          {VIEWS.map((view) => (
            <span key={view} className={view === 'Tokens' ? 'is-on' : undefined}>
              {view}
            </span>
          ))}
        </div>
        <p className="ti-note">
          Personal workspace
          <br />
          Usage metadata only
        </p>
      </aside>
      <div>
        <div className="ti-dhead">
          <strong>Tokens</strong>
          <span className="ti-tag">Demo data</span>
          <span className="ti-grow" />
          <span className="ti-mach">
            <b>mock.tokeninsights.local</b>
            <br />
            LOCAL, connected
          </span>
          <span className="ti-ctl">Refresh</span>
          <span className="ti-dot" />
        </div>
        <div className="ti-filters">
          <span>Filters</span>
          {FILTERS.map((filter) => (
            <span key={filter} className="ti-ctl">
              {filter}
            </span>
          ))}
        </div>
        <div className="ti-reads">
          {READOUTS.map((readout) => (
            <div key={readout.label}>
              <span>{readout.label}</span>
              <b>{readout.value}</b>
              <small>{readout.detail}</small>
            </div>
          ))}
        </div>
        <div className="ti-notice">
          12K ambiguous usage excluded. <u>Review excluded usage</u>
        </div>
        <div className="ti-sec">
          <div className="ti-sh">
            <h3>Usage over time</h3>
            <div className="ti-seg">
              {METRICS.map((metric) => (
                <span key={metric} className={metric === 'Total' ? 'is-on' : undefined}>
                  {metric}
                </span>
              ))}
            </div>
          </div>
          <UsageChart />
        </div>
        <div className="ti-sec">
          <div className="ti-sh">
            <h3>
              Tokens breakdown <span className="ti-tag">14</span>
            </h3>
            <span className="ti-ctl">Columns</span>
          </div>
          <div className="ti-table-wrap">
            <table className="ti-table">
              <thead>
                <tr>
                  <th>Period</th>
                  {COLUMNS.map((column) => (
                    <th key={column} className="ti-num">
                      {column}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {ROWS.map((row) => (
                  <tr key={row.period}>
                    <td>{row.period}</td>
                    {row.cells.map((cell, index) => (
                      <td key={COLUMNS[index]} className="ti-num">
                        {cell}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="ti-sum">
            Sessions <b>38</b> shown / <b>41</b> synced, <b>14</b> rows, Total <b>647K</b>
          </div>
        </div>
      </div>
    </div>
  )
}
