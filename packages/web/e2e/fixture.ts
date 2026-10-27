import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { test as base } from '@playwright/test'
import { z } from 'zod'

export const metadataSchema = z.object({
  localOrigin: z.url(),
  hostedOrigin: z.url(),
  hostedBackend: z.url(),
  hostedHome: z.string(),
  hostedPassword: z.string(),
})

export function runDirectory() {
  const directory = process.env.TOKENINSIGHTS_E2E_RUN
  if (!directory) throw new Error('Missing browser run directory')
  return directory
}

export function fixtureMetadata() {
  return metadataSchema.parse(
    JSON.parse(readFileSync(join(runDirectory(), 'fixture.json'), 'utf8')),
  )
}

export const test = base.extend({
  baseURL: async ({ browserName: _browserName }, use) => {
    await use(fixtureMetadata().localOrigin)
  },
})
export { expect } from '@playwright/test'
