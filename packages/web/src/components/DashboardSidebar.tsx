import { Link } from '@tanstack/react-router'
import { Activity, Box, ChartNoAxesCombined, Command, GitBranch, Layers3 } from 'lucide-react'
import type { Tab } from '../contracts'
import { labels } from '../format'
import { reduceQuery, searchFromQuery, useDashboardQuery } from '../state'
import { Brand } from './Brand'
import { Button } from './ui/button'

const tabs: { id: Tab; icon: typeof Activity }[] = [
  { id: 'tokens', icon: Activity },
  { id: 'models', icon: Box },
  { id: 'providers', icon: Layers3 },
  { id: 'harnesses', icon: Command },
  { id: 'sessions', icon: ChartNoAxesCombined },
  { id: 'context', icon: Layers3 },
  { id: 'repo', icon: GitBranch },
]

export function DashboardSidebar({
  localMachine,
  onSignOut,
}: {
  localMachine: boolean
  onSignOut?: () => Promise<void>
}) {
  const { query } = useDashboardQuery()
  return (
    <aside className="dashboard-sidebar" aria-label="Dashboard navigation">
      <Brand />
      <nav className="view-tabs" aria-label="Analytics views">
        {tabs.map(({ id, icon: Icon }) => (
          <Button key={id} variant="ghost" asChild>
            <Link
              to="/$tab"
              params={{ tab: id }}
              search={searchFromQuery(reduceQuery(query, { type: 'tab', value: id }))}
              activeOptions={{ exact: true, includeSearch: false }}
            >
              <Icon size="1em" aria-hidden="true" />
              {labels[id]}
            </Link>
          </Button>
        ))}
      </nav>
      <div className="sidebar-footer">
        <p>{localMachine ? 'Local workspace' : 'Hosted workspace'}</p>
        <p>Usage metadata only</p>
        {onSignOut && (
          <Button variant="ghost" size="sm" onClick={() => void onSignOut()}>
            Sign out
          </Button>
        )}
      </div>
    </aside>
  )
}
