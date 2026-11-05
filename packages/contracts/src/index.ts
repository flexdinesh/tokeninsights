import * as generated from './generated/api.ts'
import type { ViewerConnection } from './connection.ts'

export const periodSchema = generated.Period
export const bucketSchema = generated.Bucket
export const tabSchema = generated.UsageTab
export const sortSchema = generated.SortField
export const locationGroupSchema = generated.LocationGroup
export const selectionSchema = generated.Selection
export const rowSchema = generated.UsageRow
export const summarySchema = generated.UsageSummary
export const dashboardSchema = generated.UsageResponseV2
export const bootstrapSchema = generated.InstanceResponseV2.refine(
  (response) => response.dataReadiness !== 'ready' || response.dataEpoch.length > 0,
  { message: 'Ready server response requires a database identity', path: ['dataEpoch'] },
)
export const statusSchema = generated.StatusResponseV2.refine(
  (response) => response.dataReadiness !== 'ready' || response.dataEpoch.length > 0,
  { message: 'Ready server response requires a database identity', path: ['dataEpoch'] },
)
export const facetsSchema = generated.UsageFacetsResponseV2
export const errorSchema = generated.ErrorResponse

export type Selection = generated.SelectionOutput
export type Tab = generated.UsageTabOutput
export type Sort = generated.SortFieldOutput
export type LocationGroup = generated.LocationGroupOutput
export type Row = generated.UsageRowOutput
export type Dashboard = generated.UsageResponseV2Output
export type Bootstrap = generated.InstanceResponseV2Output
export type SyncStatus = generated.StatusResponseV2Output
export type Facets = generated.UsageFacetsResponseV2Output
export type Dimension = 'providers' | 'models' | 'harnesses' | 'sessions'

export const collectorProgressSchema = generated.CollectorProgressResponse
export type CollectorProgress = generated.CollectorProgressResponseOutput

export interface DashboardQuery extends Selection {
  dateMode?: 'calendar'
  quality: 'confirmed' | 'estimated'
  tab: Tab
  locationGroup: LocationGroup
  repositories: string[]
  directories: string[]
  sort: Sort
  direction: 'asc' | 'desc'
  page: number
  pageSize: number
}

// Operations consume semantic values. Adapters own routes, serialization and validation.
export interface DashboardClient {
  getInstance(signal?: AbortSignal): Promise<Bootstrap>
  getStatus(signal?: AbortSignal): Promise<SyncStatus>
  getUsage(query: DashboardQuery, signal?: AbortSignal): Promise<Dashboard>
  getFacets(query: DashboardQuery, search: string, signal?: AbortSignal): Promise<Facets>
  getCollectorProgress(signal?: AbortSignal): Promise<CollectorProgress>
  getConnection(signal?: AbortSignal): Promise<ViewerConnection>
  refresh(signal?: AbortSignal): Promise<void>
}

export { viewerConnectionSchema } from './connection.ts'
export type { ViewerConnection } from './connection.ts'

export { localBackendDescriptorSchema } from './local.ts'
export type { LocalBackendDescriptor } from './local.ts'
