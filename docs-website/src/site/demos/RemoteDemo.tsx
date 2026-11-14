'use client'

import { Stage } from '../../stageshow/stage'
import type { Act } from '../../stageshow/act'
import { Caret, Prompt, Window } from '../../stageshow/window'
import { AppSidebar, Connection, Loaded, Readout, RefreshButton } from './app'
import { UsageChart } from './UsageChart'
import './remote.css'

// Remote section, two connections to devbox from the laptop's terminal. First
// `tokeninsights connect ssh://you@devbox` is typed; then the connect command
// that `tokeninsights remote` printed on devbox is pasted. Each opens a desktop
// window on devbox's backend whose header names the host and transport, and
// devbox's own usage loads into it. Cmd+W closes the window and Cmd+K clears
// the terminal, so the loop returns to its opening frame with a seen cause.

type Transport = 'ssh' | 'tcp'
type S = { transport: Transport; typed: string; sent: boolean; open: boolean; ready: boolean }

const initial: S = { transport: 'ssh', typed: '', sent: false, open: false, ready: false }
const POSTER: S = { transport: 'ssh', typed: '', sent: true, open: true, ready: true }

// The pin and token are random demo values, never a live credential
const TCP_ADDRESS = '10.0.0.12:41827'
const TCP_PIN = 'b2f8fee8ecb641d45c79e36cddef7bb25573c71e7187cc1431c4573080215c64'
const TCP_TOKEN = 'D83IDaj399itAGANvdQOY87_F9E1juRIav_gyUNYqm8'
const COMMANDS: Record<Transport, string> = {
  ssh: 'tokeninsights connect ssh://you@devbox',
  tcp: `tokeninsights connect 'tokeninsights://connect/v1?address=${TCP_ADDRESS}#pin=${TCP_PIN}&token=${TCP_TOKEN}'`,
}
const LINES: Record<Transport, string> = {
  ssh: 'SSH · ssh://you@devbox · connected',
  tcp: `TCP · tcp://${TCP_ADDRESS} · connected`,
}
const WIDE = { width: 760, height: 470 }
const COMPACT = { below: 640, width: 358, height: 350 }
const REST = { wide: { x: 56, y: 260 }, compact: { x: 300, y: 200 } }
const GRID_K = [0, 50, 100, 150]

type Machine = {
  hostname: string
  line: string
  totals: { label: string; value: string }[]
  daysK: number[]
  rows: { period: string; sessions: string; total: string }[]
}

const LAPTOP: Machine = {
  hostname: 'laptop',
  line: 'LOCAL · local · connected',
  totals: [
    { label: 'Total tokens', value: '647K' },
    { label: 'Input', value: '369K' },
    { label: 'Output', value: '78K' },
  ],
  daysK: [31, 44, 38, 52, 29, 18, 47, 63, 71, 58, 40, 22, 66, 68],
  rows: [
    { period: '2026-09-14', sessions: '4', total: '68K' },
    { period: '2026-09-13', sessions: '5', total: '66K' },
  ],
}
const DEVBOX: Machine = {
  hostname: 'devbox',
  line: LINES.ssh,
  totals: [
    { label: 'Total tokens', value: '1.4M' },
    { label: 'Input', value: '812K' },
    { label: 'Output', value: '164K' },
  ],
  daysK: [84, 96, 71, 118, 104, 52, 61, 122, 139, 117, 90, 76, 128, 112],
  rows: [
    { period: '2026-09-14', sessions: '9', total: '112K' },
    { period: '2026-09-13', sessions: '11', total: '128K' },
  ],
}
// Compact windows run off the right edge; their figures stay inside the crop
const COMPACT_CHART_WIDTH = 300
const COMPACT_READOUTS = 2
const SKELETON_WIDTH = { hostname: 48, value: 56, cell: 28 }

const dayLabel = (day: number) => `09-${String(day + 1).padStart(2, '0')}`

async function connect(a: Act<S>, transport: Transport) {
  if (transport === 'ssh') await a.type('typed', COMMANDS.ssh, { interval: 45 })
  else {
    // The command `tokeninsights remote` printed on devbox, pasted whole
    await a.key('cmd+v')
    await a.set({ typed: COMMANDS.tcp })
  }
  await a.wait(450)
  await a.key('enter')
  await a.set({ sent: true, open: true })
  await a.move(REST[a.layout])
  await a.wait(1000)
  await a.set({ ready: true })
  await a.wait(2800)
  await a.key('cmd+w')
  await a.set({ open: false })
  await a.wait(400)
  // Closed and faded: the next window loads from scratch
  await a.snap({ ready: false })
}

/** Focus the terminal and clear it for the next connection. */
async function clear(a: Act<S>, next: Transport) {
  await a.click({ at: 'terminal', x: 0.7, y: 0.75 })
  await a.wait(200)
  await a.key('cmd+k')
  await a.set({ transport: next, typed: '', sent: false })
  await a.wait(300)
}

async function script(a: Act<S>) {
  await a.wait(700)
  await connect(a, 'ssh')
  await clear(a, 'tcp')
  await a.wait(300)
  await connect(a, 'tcp')
  await clear(a, 'ssh')
  await a.move(REST[a.layout])
  await a.wait(400)
}

function AppWindow({
  machine,
  ready,
  compact,
  className,
  receded,
}: {
  machine: Machine
  ready: boolean
  compact: boolean
  className: string
  receded?: boolean
}) {
  const connection = (
    <Connection
      hostname={machine.hostname}
      line={ready ? machine.line : 'Connecting…'}
      hostnameReady={ready}
      hostnameWidth={SKELETON_WIDTH.hostname}
    />
  )
  const body = (
    <>
      <div className="ti-app-reads ti-remote-reads">
        {(compact ? machine.totals.slice(0, COMPACT_READOUTS) : machine.totals).map((total) => (
          <Readout
            key={total.label}
            label={total.label}
            value={
              <Loaded ready={ready} width={SKELETON_WIDTH.value}>
                {total.value}
              </Loaded>
            }
          />
        ))}
      </div>
      <div className="ti-app-sec ti-remote-chart" data-ready={ready || undefined}>
        <div className="ti-app-sh">
          <span>Usage over time</span>
        </div>
        <UsageChart
          width={compact ? COMPACT_CHART_WIDTH : 422}
          height={compact ? 120 : 190}
          maxK={150}
          gridK={GRID_K}
          daysK={machine.daysK}
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
              <th>Sessions</th>
              <th>Total</th>
            </tr>
          </thead>
          <tbody>
            {machine.rows.map((row) => (
              <tr key={row.period}>
                <td>{row.period}</td>
                <td>
                  <Loaded ready={ready} width={SKELETON_WIDTH.cell} align="end">
                    {row.sessions}
                  </Loaded>
                </td>
                <td>
                  <Loaded ready={ready} width={SKELETON_WIDTH.cell} align="end">
                    {row.total}
                  </Loaded>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  )
  if (compact) {
    return (
      <Window className={className} lights="grey" lead={connection} receded={receded}>
        {body}
      </Window>
    )
  }
  return (
    <Window
      className={className}
      lights="grey"
      sidebar={<AppSidebar />}
      sidebarWidth={150}
      receded={receded}
      lead={
        <span className="ti-app-title">
          <span>/ </span>Tokens
        </span>
      }
      toolbar={
        <>
          {connection}
          <span className="ti-app-rule" />
          <RefreshButton pending={false} />
        </>
      }
    >
      {body}
    </Window>
  )
}

function Terminal({ s }: { s: S }) {
  // Enter leaves the command and opens a fresh prompt under it; Cmd+K clears
  // back to one empty prompt. Terminal output changes in whole lines.
  return (
    <Window className="ti-remote-term" chrome="terminal" lights="grey" title="laptop — zsh">
      <div className="ti-remote-term-lines" data-target="terminal" data-cursor="ibeam" data-snap>
        {s.sent && <Prompt>{COMMANDS[s.transport]}</Prompt>}
        <Prompt>
          {s.sent ? '' : s.typed}
          <Caret still={s.typed.length > 0 && !s.sent} />
        </Prompt>
      </div>
    </Window>
  )
}

function Scene({ s, compact }: { s: S; compact: boolean }) {
  return (
    <>
      <AppWindow
        machine={LAPTOP}
        ready
        compact={compact}
        className="ti-remote-back"
        receded={s.open}
      />
      <div className="ti-remote-front" data-open={s.open || undefined}>
        <AppWindow
          machine={{ ...DEVBOX, line: LINES[s.transport] }}
          ready={s.ready}
          compact={compact}
          className="ti-remote-front-window"
        />
      </div>
      <Terminal s={s} />
    </>
  )
}

export function RemoteDemo() {
  return (
    <Stage
      width={WIDE.width}
      height={WIDE.height}
      compact={COMPACT}
      initial={initial}
      poster={POSTER}
      script={script}
      cursor={{ at: REST }}
      fidelity="realistic"
      className="ti-demo ti-remote-demo"
    >
      {(s, _rect, layout) => <Scene s={s} compact={layout === 'compact'} />}
    </Stage>
  )
}
