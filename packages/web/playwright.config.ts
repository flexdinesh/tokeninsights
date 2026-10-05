import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  use: {
    baseURL: 'http://127.0.0.1:18765',
    viewport: { width: 1440, height: 1100 },
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  webServer: {
    command: 'node e2e/server.ts',
    url: 'http://127.0.0.1:18765/api/v1/instance',
    reuseExistingServer: false,
  },
})
