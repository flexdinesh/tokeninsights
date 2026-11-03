import type { DashboardQuery as QueryState } from '@tokeninsights/contracts'

export function apiQueryParams(query: QueryState): URLSearchParams {
  const params = new URLSearchParams({
    period: query.period,
    bucket: query.bucket,
    tab: query.tab,
    sort: query.sort,
    direction: query.direction,
    page: String(query.page),
    pageSize: String(query.pageSize),
  })
  if (query.quality === 'estimated') params.set('quality', 'estimated')
  if (query.from) params.set('from', query.from)
  if (query.to) params.set('to', query.to)
  for (const value of query.providers) params.append('provider', value)
  for (const value of query.models) params.append('model', value)
  for (const value of query.harnesses) params.append('harness', value)
  for (const value of query.sessions) params.append('session', value)
  if (query.tab === 'repo') {
    params.set('locationGroup', query.locationGroup)
    for (const value of query.repositories) params.append('repository', value)
    for (const value of query.directories) params.append('directory', value)
  }
  return params
}
