import * as generated from './generated/api'

export const periodSchema = generated.Period
export const bucketSchema = generated.Bucket
export const tabSchema = generated.UsageTab
export const sortSchema = generated.SortField
export const selectionSchema = generated.Selection
export const rowSchema = generated.UsageRow
export const summarySchema = generated.UsageSummary
export const dashboardSchema = generated.UsageResponse
export const bootstrapSchema = generated.InstanceResponse
export const statusSchema = generated.SyncResponse
export const facetsSchema = generated.UsageFacetsResponse
export const errorSchema = generated.ErrorResponse

export type Selection = generated.SelectionOutput
export type Tab = generated.UsageTabOutput
export type Sort = generated.SortFieldOutput
export type Row = generated.UsageRowOutput
export type Dashboard = generated.UsageResponseOutput
export type Bootstrap = generated.InstanceResponseOutput
export type SyncStatus = generated.SyncResponseOutput
export type Facets = generated.UsageFacetsResponseOutput
export type Dimension = 'providers' | 'models' | 'harnesses' | 'sessions'
