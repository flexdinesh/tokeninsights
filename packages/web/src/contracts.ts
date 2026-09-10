import { z } from 'zod'

export const periodSchema = z.enum(['today', 'yesterday', 'week', 'month', 'year', 'all'])
export const bucketSchema = z.enum(['day', 'week', 'month', 'year'])
export const tabSchema = z.enum(['tokens', 'models', 'providers', 'harnesses', 'sessions', 'context'])
export const sortSchema = z.enum(['name', 'date', 'total', 'input', 'output', 'reasoning', 'cacheRead', 'cacheWrite', 'sessions', 'context', 'averageContext', 'medianContext', 'maxContext', 'harness', 'provider', 'model'])
export const selectionSchema = z.object({
  period: periodSchema, bucket: bucketSchema, from: z.string(), to: z.string(),
  providers: z.array(z.string()), models: z.array(z.string()), harnesses: z.array(z.string()), sessions: z.array(z.string()),
})
const count = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER)
export const rowSchema = z.object({
  key: z.string(), name: z.string(), harness: z.string(), provider: z.string(), model: z.string(),
  date: count, sessions: count, input: count, output: count, reasoning: count,
  cacheRead: count, cacheWrite: count, total: count, context: count,
  averageContext: count, medianContext: count, maxContext: count,
})
export const summarySchema = z.object({ total: count, input: count, output: count, reasoning: count, cacheRead: count, cacheWrite: count, sessions: count, syncedSessions: count })
export const dashboardSchema = z.object({
  rows: z.array(rowSchema), chart: z.array(rowSchema), rowCount: count, page: count, pageSize: count,
  summary: summarySchema, lastSynced: count, range: z.string(),
})
export const bootstrapSchema = z.object({ defaults: selectionSchema, hostname: z.string(), timezone: z.string() })
export const statusSchema = z.object({ running: z.boolean(), phase: z.string(), harnesses: z.record(z.string(), z.string()), error: z.string(), revision: count })
export const facetsSchema = z.object({ providers: z.array(z.string()), models: z.array(z.string()), harnesses: z.array(z.string()), sessions: z.array(z.string()) })

export type Selection = z.infer<typeof selectionSchema>
export type Tab = z.infer<typeof tabSchema>
export type Sort = z.infer<typeof sortSchema>
export type Row = z.infer<typeof rowSchema>
export type Dashboard = z.infer<typeof dashboardSchema>
export type Bootstrap = z.infer<typeof bootstrapSchema>
export type SyncStatus = z.infer<typeof statusSchema>
export type Facets = z.infer<typeof facetsSchema>
export type Dimension = 'providers' | 'models' | 'harnesses' | 'sessions'
