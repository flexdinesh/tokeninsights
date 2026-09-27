import { ArrowDownToLine, LoaderCircle, Monitor, Moon, RefreshCw, Sun } from 'lucide-react'
import { useDashboardPreferences } from '../state'
import { Button } from './ui/button'

export function DashboardHeader({
  hostname,
  running,
  serverUnavailable,
  lastSynced,
  reloading,
  onReload,
  onSync,
}: {
  hostname: string
  running: boolean
  serverUnavailable: boolean
  lastSynced?: number
  reloading: boolean
  onReload: () => void
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
        <Button
          variant="outline"
          size="sm"
          className="reload-button"
          aria-label="Reload Data"
          title="Reload Data"
          disabled={running}
          onClick={onReload}
        >
          <RefreshCw size="1em" className={reloading ? 'spin' : ''} />
          <span>Reload Data</span>
        </Button>
        <Button className="sync-button" size="sm" disabled={running} onClick={onSync}>
          {running ? <LoaderCircle size="1em" className="spin" /> : <ArrowDownToLine size="1em" />}
          <span>{running ? 'Syncing…' : 'Sync Usage'}</span>
        </Button>
      </div>
    </header>
  )
}
