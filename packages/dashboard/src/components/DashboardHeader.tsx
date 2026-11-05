import { Monitor, Moon, RotateCw, Sun } from 'lucide-react'
import { useDashboardPreferences, useDashboardQuery } from '../state'
import { labels } from '../format'
import type { CollectorProgress, SyncStatus, ViewerConnection } from '@tokeninsights/contracts'
import { DashboardStatus } from './DashboardStatus'
import type { DashboardStatusValue } from './DashboardStatus'
import { Button, HeaderDivider, PageHeader } from '../ui'

export function DashboardHeader({
  hostname,
  instanceId,
  connection,
  connectionUnavailable,
  timezone,
  status,
  progress,
  processUsage,
  lastSynced,
  refreshPending,
  onRefresh,
}: {
  hostname: string
  instanceId: string
  connection: ViewerConnection | undefined
  connectionUnavailable: boolean
  timezone: string
  status: DashboardStatusValue
  progress: CollectorProgress | undefined
  processUsage: SyncStatus['processUsage']
  lastSynced?: number
  refreshPending: boolean
  onRefresh: () => void
}) {
  const { theme, setTheme } = useDashboardPreferences()
  const { query } = useDashboardQuery()
  const connectionState = connectionUnavailable ? 'unavailable' : connection?.state
  return (
    <PageHeader title={`/ ${labels[query.tab]}`}>
      <div className="min-w-0 text-label" role="group" aria-label="Backend connection">
        <span
          className="block max-w-(--server-identity-width) truncate font-semibold"
          title={`Backend hostname: ${hostname} · Identity: ${instanceId}`}
        >
          {hostname}
        </span>
        <span className="block wrap-anywhere" role="status" aria-label="Connection state">
          {connection
            ? `${connection.transport.toUpperCase()} · ${connection.target} · ${connectionState}`
            : connectionUnavailable
              ? 'Connection details unavailable'
              : 'Connecting…'}
        </span>
      </div>
      <HeaderDivider />
      <Button variant="outline" size="compact" disabled={refreshPending} onClick={onRefresh}>
        <RotateCw aria-hidden="true" />
        {refreshPending ? 'Refreshing…' : 'Refresh'}
      </Button>
      <DashboardStatus
        instanceId={instanceId}
        status={status}
        progress={progress}
        processUsage={processUsage}
        lastSynced={lastSynced}
        timezone={timezone}
      />
      <Button
        variant="ghost"
        size="icon"
        title={`Theme: ${theme}`}
        aria-label={`Theme: ${theme}. Change theme`}
        onClick={() =>
          setTheme(theme === 'system' ? 'light' : theme === 'light' ? 'dark' : 'system')
        }
      >
        {theme === 'dark' ? (
          <Moon aria-hidden="true" />
        ) : theme === 'light' ? (
          <Sun aria-hidden="true" />
        ) : (
          <Monitor aria-hidden="true" />
        )}
      </Button>
    </PageHeader>
  )
}
