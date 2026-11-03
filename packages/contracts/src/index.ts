import * as generated from './generated/api.ts'

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

export const authInfoSchema = generated.AuthInfo
export const accountUserSchema = generated.AccountUser
export const apiKeyListSchema = generated.APIKeyList
export const apiKeyCreatedSchema = generated.APIKeyCreated
export type AccountUser = generated.AccountUserOutput
export type APIKeyInfo = generated.APIKeyInfoOutput
export type APIKeyCreated = generated.APIKeyCreatedOutput
export const apiKeyLabelMaxCharacters = generated.createAPIKeyRequestLabelMax
export const browserSessionPasswordMinCharacters = generated.browserSessionRequestPasswordMin
export const browserSessionPasswordMaxCharacters = generated.browserSessionRequestPasswordMax
export const newPasswordMinCharacters = generated.changePasswordRequestPasswordMin
export const newPasswordMaxCharacters = generated.changePasswordRequestPasswordMax
export type CreateAPIKeyRequest = generated.CreateAPIKeyRequest

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

export type Credentials = { username: string; password: string }

// Operations consume semantic values. Adapters own routes, serialization and validation.
export interface DashboardClient {
  getInstance(signal?: AbortSignal): Promise<Bootstrap>
  getStatus(signal?: AbortSignal): Promise<SyncStatus>
  getUsage(query: DashboardQuery, signal?: AbortSignal): Promise<Dashboard>
  getFacets(query: DashboardQuery, search: string, signal?: AbortSignal): Promise<Facets>
  getCollectorProgress(signal?: AbortSignal): Promise<CollectorProgress>
  getAuthInfo(signal?: AbortSignal): Promise<generated.AuthInfoOutput>
  getAccount(datasetId: string, signal?: AbortSignal): Promise<AccountUser>
  getAPIKeys(datasetId: string, signal?: AbortSignal): Promise<generated.APIKeyListOutput>
  changeBrowserSession(credentials?: Credentials): Promise<void>
  signup(credentials: Credentials): Promise<void>
  changePassword(datasetId: string, currentPassword: string, password: string): Promise<void>
  createAPIKey(datasetId: string, request: CreateAPIKeyRequest): Promise<APIKeyCreated>
  revokeAPIKey(datasetId: string, tokenId: string): Promise<void>
}

export class AuthenticationRequired extends Error {
  constructor() {
    super('Sign in to view your token usage.')
    this.name = 'AuthenticationRequired'
  }
}

export { localBackendDescriptorSchema } from './local.ts'
export type { LocalBackendDescriptor } from './local.ts'
