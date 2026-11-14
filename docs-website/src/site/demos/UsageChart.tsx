// Tokens per day as a line over a soft area. A change in the series eases the
// path from the old shape to the new one (CSS `d`), and the end dot rides it.

const PAD = { left: 34, right: 8, top: 8, bottom: 20 }
const DAY_LABEL_STEP = 3

export function UsageChart(props: UsageChart.Props) {
  const { width, height, maxK, gridK, daysK } = props
  const last = daysK.length - 1
  const x = (day: number) => PAD.left + (day * (width - PAD.left - PAD.right)) / last
  const y = (valueK: number) => PAD.top + (height - PAD.bottom - PAD.top) * (1 - valueK / maxK)
  const line = daysK.map((value, day) => `${day === 0 ? 'M' : 'L'}${x(day)} ${y(value)}`).join(' ')
  const area = `${line} L${x(last)} ${y(0)} L${x(0)} ${y(0)} Z`
  const labelled = daysK.map((_, day) => day).filter((day) => (last - day) % DAY_LABEL_STEP === 0)
  return (
    <svg
      className="ti-app-chart"
      viewBox={`0 0 ${width} ${height}`}
      style={{ '--ti-line-d': `path("${line}")`, '--ti-area-d': `path("${area}")` }}
      aria-hidden
    >
      {gridK.map((value) => (
        <g key={value}>
          <line x1={PAD.left} x2={width - PAD.right} y1={y(value)} y2={y(value)} />
          <text x={PAD.left - 6} y={y(value) + 3.5} textAnchor="end">
            {value === 0 ? '0' : `${value}K`}
          </text>
        </g>
      ))}
      {labelled.map((day) => (
        <text
          key={day}
          x={x(day)}
          y={height - 5}
          textAnchor={day === 0 ? 'start' : day === last ? 'end' : 'middle'}
        >
          {props.dayLabel(day)}
        </text>
      ))}
      <path className="ti-app-area" />
      <path className="ti-app-line" />
      <circle
        className="ti-app-end"
        r="3"
        cx="0"
        cy="0"
        style={{ transform: `translate(${x(last)}px, ${y(daysK[last] ?? 0)}px)` }}
      />
    </svg>
  )
}

export declare namespace UsageChart {
  type Props = {
    width: number
    height: number
    maxK: number
    gridK: number[]
    daysK: number[]
    dayLabel: (day: number) => string
  }
}
