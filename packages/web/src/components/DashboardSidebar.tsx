import { Link } from '@tanstack/react-router'
import {
  Activity,
  Box,
  ChartNoAxesCombined,
  Command,
  Gauge,
  GitBranch,
  Layers3,
} from 'lucide-react'
import type { Tab } from '../contracts'
import { labels } from '../format'
import { reduceQuery, searchFromQuery } from '../query'
import type { QueryState } from '../query'
import { Button, Sidebar, SidebarItem } from '@/ui'

const tabs: { id: Tab; icon: typeof Activity }[] = [
  { id: 'tokens', icon: Activity },
  { id: 'models', icon: Box },
  { id: 'providers', icon: Layers3 },
  { id: 'harnesses', icon: Command },
  { id: 'sessions', icon: ChartNoAxesCombined },
  { id: 'context', icon: Gauge },
  { id: 'repo', icon: GitBranch },
]

export function DashboardSidebar({
  localMachine,
  onSignOut,
  query,
}: {
  localMachine: boolean
  onSignOut?: () => Promise<void>
  query: QueryState
}) {
  return (
    <Sidebar
      label="Dashboard navigation"
      navLabel="Analytics views"
      note={
        <>
          <p>{localMachine ? 'Local workspace' : 'Hosted workspace'}</p>
          <p>Usage metadata only</p>
        </>
      }
      actions={
        onSignOut && (
          <Button variant="quiet" size="compact" onClick={() => void onSignOut()}>
            Sign out
          </Button>
        )
      }
    >
      {tabs.map(({ id, icon: Icon }) => (
        <SidebarItem key={id}>
          <Link
            to="/$tab"
            params={{ tab: id }}
            search={searchFromQuery(reduceQuery(query, { type: 'tab', value: id }))}
            activeOptions={{ exact: true, includeSearch: false }}
          >
            <Icon aria-hidden="true" />
            {labels[id]}
          </Link>
        </SidebarItem>
      ))}
    </Sidebar>
  )
}
