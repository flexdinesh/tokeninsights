import { lazy, Suspense } from 'react'
import { CatchBoundary } from '@tanstack/react-router'
import type { Row } from '../contracts'
import { apiQueryParams, useDashboardQuery } from '../state'
import { Alert, AlertDescription } from './ui/alert'
import { Button } from './ui/button'
import { Skeleton } from './ui/skeleton'

const UsageChart = lazy(() =>
  import('./UsageChart').then((module) => ({ default: module.UsageChart })),
)

function ChartError() {
  return (
    <Alert className="error-banner" variant="destructive" role="alert">
      <AlertDescription>Chart couldn’t load. Reload the page to try again.</AlertDescription>
      <Button variant="outline" onClick={() => window.location.reload()}>
        Reload page
      </Button>
    </Alert>
  )
}

export function ChartPanel({ rows, totalTokens }: { rows: Row[]; totalTokens: number }) {
  const { query } = useDashboardQuery()
  const resetKey = apiQueryParams(query).toString()
  return (
    <CatchBoundary getResetKey={() => resetKey} errorComponent={ChartError}>
      <Suspense
        fallback={<Skeleton className="skeleton-chart" role="status" aria-label="Loading chart" />}
      >
        <UsageChart rows={rows} totalTokens={totalTokens} />
      </Suspense>
    </CatchBoundary>
  )
}
