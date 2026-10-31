import { randomUUID } from 'node:crypto'
import { resolve } from 'node:path'
import { defineConfig } from '@playwright/test'

// Inherited by workers and setup; simultaneous invocations never share output.
const runDirectory = process.env.TOKENINSIGHTS_E2E_RUN ?? resolve('test-results', randomUUID())
process.env.TOKENINSIGHTS_E2E_RUN = runDirectory

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  workers: 1,
  reporter: 'dot',
  outputDir: resolve(runDirectory, 'artifacts'),
  globalSetup: './e2e/server.ts',
  use: {
    viewport: { width: 1440, height: 1100 },
    ignoreHTTPSErrors: true,
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
})
