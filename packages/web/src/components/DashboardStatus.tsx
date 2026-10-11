import { useEffect, useRef, useState } from 'react'
import { Activity, CircleAlert, LoaderCircle } from 'lucide-react'
import type { CollectorProgress as Progress, SyncStatus } from '../contracts'
import { formatServerDateTime, serverTimeZoneLabel } from '../format'
import { CollectorProgress } from './CollectorProgress'
import { Button } from './ui/button'
import { Popover, PopoverContent, PopoverTrigger } from './ui/popover'

export type DashboardStatusValue = {
  label: string
  state: 'ready' | 'loading' | 'attention'
}

const hoverCloseDelayMs = 150

export function DashboardStatus({
  status,
  progress,
  processUsage,
  localMachine,
  lastSynced,
  timezone,
}: {
  status: DashboardStatusValue
  progress: Progress | undefined
  processUsage: SyncStatus['processUsage']
  localMachine: boolean
  lastSynced?: number
  timezone: string
}) {
  const [open, setOpen] = useState(false)
  const closeTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const pinned = useRef(false)
  useEffect(() => () => clearTimeout(closeTimer.current), [])

  function reveal() {
    clearTimeout(closeTimer.current)
    setOpen(true)
  }

  function leave() {
    if (pinned.current) return
    closeTimer.current = setTimeout(() => setOpen(false), hoverCloseDelayMs)
  }

  const statusScope = localMachine ? 'Local' : 'Dashboard'
  return (
    <Popover
      open={open}
      onOpenChange={(value) => {
        clearTimeout(closeTimer.current)
        pinned.current = value
        setOpen(value)
      }}
    >
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className={`loading-status loading-status-${status.state}`}
          aria-label={`${statusScope} status: ${status.label}`}
          onPointerEnter={(event) => {
            if (event.pointerType === 'mouse') reveal()
          }}
          onPointerLeave={leave}
          onFocus={() => {
            pinned.current = true
            reveal()
          }}
          onClick={(event) => {
            event.preventDefault()
            pinned.current = true
            reveal()
          }}
        >
          {status.state === 'loading' ? (
            <LoaderCircle className="spin" aria-hidden="true" />
          ) : status.state === 'attention' ? (
            <CircleAlert aria-hidden="true" />
          ) : (
            <Activity aria-hidden="true" />
          )}
          <span className="sr-only" role="status" aria-label={status.label}>
            {status.label}
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="loading-status-details"
        aria-label={`${statusScope} status details`}
        onPointerEnter={reveal}
        onPointerLeave={leave}
        onOpenAutoFocus={(event) => event.preventDefault()}
        onCloseAutoFocus={(event) => event.preventDefault()}
      >
        {localMachine && (
          <div className="local-resource-usage" role="group" aria-label="Process resource usage">
            <span title="TokenInsights process CPU · 100% = one core">
              CPU{' '}
              {processUsage?.cpuPercent === undefined
                ? '—'
                : `${processUsage.cpuPercent.toFixed(1)}%`}
            </span>
            <span title="TokenInsights resident memory">
              RAM{' '}
              {processUsage?.residentMemoryBytes === undefined
                ? '—'
                : formatMemory(processUsage.residentMemoryBytes)}
            </span>
          </div>
        )}
        <p>{status.label}</p>
        {lastSynced ? (
          <p>
            <time
              dateTime={new Date(lastSynced).toISOString()}
              title={serverTimeZoneLabel(timezone)}
            >
              Last ingestion {formatServerDateTime(lastSynced, timezone)}
            </time>
          </p>
        ) : null}
        {localMachine && <CollectorProgress progress={progress} />}
      </PopoverContent>
    </Popover>
  )
}

const bytesPerMiB = 1024 * 1024
function formatMemory(bytes: number): string {
  return `${(bytes / bytesPerMiB).toLocaleString(undefined, { maximumFractionDigits: 1 })} MiB`
}
