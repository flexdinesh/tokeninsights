import { useEffect, useRef, useState } from 'react'
import { Check, CircleAlert, LoaderCircle } from 'lucide-react'
import type { CollectorProgress as Progress } from '../contracts'
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
}: {
  status: DashboardStatusValue
  progress: Progress | undefined
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
          aria-label={`Loading status: ${status.label}`}
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
            <Check aria-hidden="true" />
          )}
          <span className="sr-only" role="status" aria-label={status.label}>
            {status.label}
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="loading-status-details"
        aria-label="Loading status details"
        onPointerEnter={reveal}
        onPointerLeave={leave}
        onOpenAutoFocus={(event) => event.preventDefault()}
        onCloseAutoFocus={(event) => event.preventDefault()}
      >
        <h2>Loading status</h2>
        <p>{status.label}</p>
        <CollectorProgress progress={progress} />
      </PopoverContent>
    </Popover>
  )
}
