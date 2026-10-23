import { Monitor, Moon, Sun, UserRound } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { useDashboardPreferences, useDashboardQuery } from '../state'
import { labels } from '../format'
import type { CollectorProgress, SyncStatus } from '../contracts'
import { DashboardStatus } from './DashboardStatus'
import type { DashboardStatusValue } from './DashboardStatus'
import { Button, HeaderDivider, PageHeader } from '@/ui'

export function DashboardHeader({
  hostname,
  timezone,
  status,
  progress,
  processUsage,
  localMachine,
  lastSynced,
}: {
  hostname: string
  timezone: string
  status: DashboardStatusValue
  progress: CollectorProgress | undefined
  processUsage: SyncStatus['processUsage']
  localMachine: boolean
  lastSynced?: number
}) {
  const { theme, setTheme } = useDashboardPreferences()
  const { query } = useDashboardQuery()
  return (
    <PageHeader title={`/ ${labels[query.tab]}`}>
      <span
        className="max-w-(--server-identity-width) min-w-0 truncate text-label font-semibold"
        title={
          localMachine
            ? `Local machine running this dashboard · ${window.location.origin}`
            : window.location.origin
        }
      >
        {hostname}
      </span>
      <HeaderDivider />
      <DashboardStatus
        status={status}
        progress={progress}
        processUsage={processUsage}
        localMachine={localMachine}
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
      {!localMachine && (
        <Button variant="ghost" size="icon" asChild>
          <Link
            to="/settings"
            search={{
              providers: [],
              models: [],
              harnesses: [],
              sessions: [],
              repositories: [],
              directories: [],
            }}
            aria-label="Account settings"
            title="Account settings"
          >
            <UserRound aria-hidden="true" />
          </Link>
        </Button>
      )}
    </PageHeader>
  )
}
