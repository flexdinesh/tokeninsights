import { Monitor, Moon, Sun, UserRound } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { useDashboardPreferences, useDashboardQuery } from '../state'
import { labels } from '../format'
import type { CollectorProgress, SyncStatus } from '../contracts'
import { DashboardStatus } from './DashboardStatus'
import type { DashboardStatusValue } from './DashboardStatus'
import { Button } from '@/ui'

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
    <header className="app-header">
      <h1 className="header-view">/ {labels[query.tab]}</h1>
      <div className="header-actions">
        <span
          className="server-identity"
          title={
            localMachine
              ? `Local machine running this dashboard · ${window.location.origin}`
              : window.location.origin
          }
        >
          {hostname}
        </span>
        <span className="header-divider" />
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
          className="theme-button"
          title={`Theme: ${theme}`}
          aria-label={`Theme: ${theme}. Change theme`}
          onClick={() =>
            setTheme(theme === 'system' ? 'light' : theme === 'light' ? 'dark' : 'system')
          }
        >
          {theme === 'dark' ? (
            <Moon size="1.1em" />
          ) : theme === 'light' ? (
            <Sun size="1.1em" />
          ) : (
            <Monitor size="1.1em" />
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
              <UserRound size="1.1em" />
            </Link>
          </Button>
        )}
      </div>
    </header>
  )
}
