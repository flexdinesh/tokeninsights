import type { ReactNode } from 'react'
import {
  Activity,
  Box,
  ChartNoAxesCombined,
  Command,
  Gauge,
  GitBranch,
  Layers3,
  RotateCw,
  type LucideIcon,
} from 'lucide-react'
import { Button } from '../../stageshow/controls/Button'
import { States } from '../../stageshow/motion/States'
import { Roll } from '../../stageshow/motion/Roll'
import './app.css'

// The TokenInsights desktop app, drawn for the landing page demos. Kept in step
// with packages/dashboard by hand: this site has no dependency on the app.

const VIEWS: { label: string; icon: LucideIcon }[] = [
  { label: 'Tokens', icon: Activity },
  { label: 'Models', icon: Box },
  { label: 'Providers', icon: Layers3 },
  { label: 'Harnesses', icon: Command },
  { label: 'Sessions', icon: ChartNoAxesCombined },
  { label: 'Context', icon: Gauge },
  { label: 'Repo', icon: GitBranch },
]

export function AppSidebar(props: AppSidebar.Props) {
  return (
    <div className="ti-app-side">
      <img className="ti-app-lockup ti-app-lockup--light" src="/lockup-light.svg" alt="" />
      <img className="ti-app-lockup ti-app-lockup--dark" src="/lockup-dark.svg" alt="" />
      <div className="ti-app-nav">
        {VIEWS.map(({ label, icon: Icon }) => (
          <span key={label} data-on={label === 'Tokens' || undefined}>
            <Icon className="ti-app-icon" aria-hidden />
            {label}
          </span>
        ))}
      </div>
      {props.note && (
        <span className="ti-app-note">
          Personal workspace
          <br />
          Usage metadata only
        </span>
      )}
    </div>
  )
}

export declare namespace AppSidebar {
  type Props = { note?: boolean }
}

/** The header's backend identity: hostname over "TRANSPORT · target · state". */
export function Connection(props: Connection.Props) {
  return (
    <span className="ti-app-conn">
      <b>
        <Loaded ready={props.hostnameReady ?? true} width={props.hostnameWidth ?? 0} align="end">
          {props.hostname}
        </Loaded>
      </b>
      <span>
        <Roll>{props.line}</Roll>
      </span>
    </span>
  )
}

export declare namespace Connection {
  type Props = { hostname: string; line: string; hostnameReady?: boolean; hostnameWidth?: number }
}

export function RefreshButton(props: RefreshButton.Props) {
  return (
    <Button
      className="ti-app-refresh"
      data-target={props.target}
      data-pending={props.pending || undefined}
      icon={<RotateCw className="ti-app-icon" aria-hidden />}
    >
      {props.pending ? 'Refreshing…' : 'Refresh'}
    </Button>
  )
}

export declare namespace RefreshButton {
  type Props = { pending: boolean; target?: string }
}

/** One readout: label, the big figure, a line of detail. */
export function Readout(props: Readout.Props) {
  return (
    <div className="ti-app-read">
      <span>{props.label}</span>
      <b>{props.value}</b>
      {props.detail !== undefined && <small>{props.detail}</small>}
    </div>
  )
}

export declare namespace Readout {
  type Props = { label: string; value: ReactNode; detail?: ReactNode }
}

/** A value that is still loading: a quiet bar holding the figure's line. */
export function Loaded(props: Loaded.Props) {
  return (
    <States
      align={props.align ?? 'start'}
      value={props.ready ? 'ready' : 'loading'}
      states={{
        loading: <span className="ti-app-skeleton" style={{ width: props.width }} />,
        ready: props.children,
      }}
    />
  )
}

export declare namespace Loaded {
  type Props = { ready: boolean; width: number; align?: 'start' | 'end'; children: ReactNode }
}
