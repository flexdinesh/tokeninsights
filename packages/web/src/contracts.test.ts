import { describe, expect, it } from 'vitest'
import { bootstrapSchema, dashboardSchema, facetsSchema, statusSchema } from './contracts'
import { mockBootstrap, mockDashboard, mockFacets, mockSyncStatus } from './mocks/data'

describe('analytics response identity', () => {
  for (const { name, schema, response } of [
    { name: 'usage', schema: dashboardSchema, response: mockDashboard('tokens') },
    { name: 'facets', schema: facetsSchema, response: mockFacets },
  ]) {
    it(`${name} requires nonempty instance/database identity and explicit revision`, () => {
      for (const key of ['instanceId', 'dataEpoch', 'revision']) {
        const absent = Object.fromEntries(
          Object.entries(response).filter(([field]) => field !== key),
        )
        expect(schema.safeParse(absent).success).toBe(false)
        expect(schema.safeParse({ ...response, [key]: null }).success).toBe(false)
        expect(schema.safeParse({ ...response, [key]: '' }).success).toBe(false)
      }
      expect(schema.safeParse({ ...response, revision: 0 }).success).toBe(true)
    })
  }
})

describe('availability response identity', () => {
  for (const { name, schema, response } of [
    { name: 'instance', schema: bootstrapSchema, response: mockBootstrap },
    { name: 'status', schema: statusSchema, response: mockSyncStatus(0) },
  ]) {
    it(`${name} requires explicit identity/readiness and refuses ready without a database`, () => {
      for (const key of ['instanceId', 'dataEpoch', 'dataReadiness']) {
        const absent = Object.fromEntries(
          Object.entries(response).filter(([field]) => field !== key),
        )
        expect(schema.safeParse(absent).success).toBe(false)
        expect(schema.safeParse({ ...response, [key]: null }).success).toBe(false)
      }
      expect(schema.safeParse({ ...response, instanceId: '' }).success).toBe(false)
      expect(schema.safeParse({ ...response, dataEpoch: '', dataReadiness: 'ready' }).success).toBe(
        false,
      )
      expect(
        schema.safeParse({ ...response, dataEpoch: '', dataReadiness: 'unavailable' }).success,
      ).toBe(true)
    })
  }
})
