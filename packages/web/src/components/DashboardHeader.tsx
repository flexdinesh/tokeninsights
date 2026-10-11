import { RefreshCw, LoaderCircle, Monitor, Moon, Sun } from 'lucide-react'
import { useDashboardPreferences, useDashboardQuery } from '../state'
import { formatServerDateTime, labels, serverTimeZoneLabel } from '../format'
import { Button } from './ui/button'

export function DashboardHeader({
  hostname,
  timezone,
  reloading,
  statusLabel,
  localMachine,
  allowReload,
  lastSynced,
  onReload,
}: {
  hostname: string
  timezone: string
  reloading: boolean
  statusLabel: string
  localMachine: boolean
  allowReload: boolean
  lastSynced?: number
  onReload: () => void
}) {
  const { theme, setTheme } = useDashboardPreferences()
  const { query } = useDashboardQuery()
  return (
    <header className="app-header">
      <span className="header-view">Analytics / {labels[query.tab]}</span>
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
        <span className="header-status" role="status">
          {statusLabel}
        </span>
        {lastSynced ? (
          <time
            className="header-status"
            dateTime={new Date(lastSynced).toISOString()}
            title={serverTimeZoneLabel(timezone)}
          >
            Last ingestion {formatServerDateTime(lastSynced, timezone)}
          </time>
        ) : null}
        <span className="header-divider" />
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
        {allowReload && (
          <Button
            variant="outline"
            className="sync-button"
            size="sm"
            disabled={reloading}
            onClick={onReload}
          >
            {reloading ? <LoaderCircle size="1em" className="spin" /> : <RefreshCw size="1em" />}
            <span>Reload</span>
          </Button>
        )}
      </div>
    </header>
  )
}
