import { ArrowDownToLine, LoaderCircle, Monitor, Moon, Sun } from 'lucide-react'
import { useDashboardPreferences } from '../state'
import { Button } from './ui/button'

export function DashboardHeader({
  hostname,
  running,
  refreshDisabled,
  pendingRefresh,
  serverUnavailable,
  lastSynced,
  onSync,
}: {
  hostname: string
  running: boolean
  refreshDisabled?: boolean
  pendingRefresh?: boolean
  serverUnavailable: boolean
  lastSynced?: number
  onSync: () => void
}) {
  const { theme, setTheme } = useDashboardPreferences()
  return (
    <header className="app-header">
      <div className="brand">
        <img src="/tokeninsights-logo.png" alt="" className="brand-mark" />
        <span>
          Token<span className="brand-light">Insights</span>
        </span>
      </div>
      <div className="header-actions">
        <span className="server-identity" title={window.location.origin}>
          {hostname}
        </span>
        <span className="header-status" role="status">
          {serverUnavailable
            ? 'Unavailable'
            : running
              ? 'Syncing…'
              : lastSynced
                ? 'Synced'
                : 'Ready'}
        </span>
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
        <Button className="sync-button" size="sm" disabled={refreshDisabled} onClick={onSync}>
          {running ? <LoaderCircle size="1em" className="spin" /> : <ArrowDownToLine size="1em" />}
          <span>{pendingRefresh ? 'Sync queued' : running ? 'Sync again' : 'Sync'}</span>
        </Button>
      </div>
    </header>
  )
}
