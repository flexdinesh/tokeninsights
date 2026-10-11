import { expect, test } from '@playwright/test'
import { mockBootstrap, mockDashboard, mockFacets, mockSyncStatus } from '../src/mocks/data'
import { bootstrapSchema } from '../src/contracts'

for (const kind of ['personal', 'hosted']) {
  test(`${kind} browser respects collector-progress capability`, async ({ page }) => {
    const progressRequests: string[] = []
    await page.route('**/api/v2/**', (route) => {
      const path = new URL(route.request().url()).pathname
      if (path.endsWith('/collector-progress')) {
        progressRequests.push(path)
        return route.fulfill({
          json: {
            instanceId: mockBootstrap.instanceId,
            attempts: [
              {
                attemptId: 'attempt',
                stage: 'submitting',
                harnesses: { codex: 'running' },
                acknowledgedBatches: 1,
                acknowledgedEntries: 12,
                pendingKnown: false,
                pending: 0,
                startedAtMs: 1000,
                updatedAtMs: 2000,
                expiresAtMs: 17000,
              },
            ],
          },
        })
      }
      const body = path.endsWith('/instance')
        ? {
            ...mockBootstrap,
            serverKind: kind,
            capabilities: [...mockBootstrap.capabilities, 'collector-progress'],
          }
        : path.endsWith('/status')
          ? {
              ...mockSyncStatus(1),
              processUsage: { cpuPercent: 12.5, residentMemoryBytes: 64 * 1024 * 1024 },
            }
          : path.endsWith('/facets')
            ? mockFacets
            : mockDashboard('tokens')
      return route.fulfill({ json: body })
    })
    await page.goto('/tokens')
    await expect(page.getByLabel('Total tokens: 647,000', { exact: true })).toBeVisible()
    if (kind === 'personal') {
      await page.getByRole('button', { name: /Local status:/ }).hover()
      await expect(page.getByRole('dialog', { name: 'Local status details' })).toBeVisible()
      await expect(page.getByRole('region', { name: 'Collector progress' })).toContainText(
        'Submitting usage',
      )
      expect(progressRequests.length).toBeGreaterThan(0)
    } else {
      await expect(page.getByRole('button', { name: /Local status:/ })).toHaveCount(0)
      await expect(page.getByRole('region', { name: 'Collector progress' })).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Reload', exact: true })).toHaveCount(0)
      expect(progressRequests).toHaveLength(0)
    }
  })
}

test('local dashboard hides Reload when capability disabled', async ({ page }) => {
  await page.route('**/api/v2/instance', async (route) => {
    const response = await route.fetch()
    const bootstrap = bootstrapSchema.parse(await response.json())
    await route.fulfill({
      json: {
        ...bootstrap,
        capabilities: bootstrap.capabilities.filter(
          (capability) => capability !== 'dashboard-reload',
        ),
      },
    })
  })
  await page.goto('/tokens')
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Reload', exact: true })).toHaveCount(0)
  await expect(page.getByTitle(/Local machine running this dashboard/)).toBeVisible()
})

test('local startup shows capture and processing before fresh totals without an empty-state flash', async ({
  page,
}) => {
  let stage = 'capturing'
  let pending = 0
  let published = false
  const methods: string[] = []
  let progressRequests = 0
  await page.route('**/api/v2/**', (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    methods.push(request.method())
    if (path.endsWith('/collector-progress')) {
      progressRequests++
      return route.fulfill({
        json: {
          instanceId: mockBootstrap.instanceId,
          attempts: [
            {
              attemptId: 'startup',
              stage,
              harnesses: { codex: stage === 'capturing' ? 'running' : 'complete' },
              acknowledgedBatches: 0,
              acknowledgedEntries: 0,
              pendingKnown: false,
              pending: 0,
              startedAtMs: 1000,
              updatedAtMs: stage === 'capturing' ? 1001 : 1002,
              expiresAtMs: 16000,
            },
          ],
        },
      })
    }
    const data = mockDashboard('tokens')
    const revision = published ? 2 : 1
    const body = path.endsWith('/instance')
      ? { ...mockBootstrap, capabilities: [...mockBootstrap.capabilities, 'collector-progress'] }
      : path.endsWith('/status')
        ? { ...mockSyncStatus(revision), pending }
        : path.endsWith('/facets')
          ? { ...mockFacets, revision }
          : published
            ? { ...data, revision }
            : {
                ...data,
                rows: [],
                chart: [],
                rowCount: 0,
                summary: {
                  total: 0,
                  input: 0,
                  output: 0,
                  cacheRead: 0,
                  cacheWrite: 0,
                  reasoning: 0,
                  sessions: 0,
                  syncedSessions: 0,
                },
              }
    return route.fulfill({ json: body })
  })
  await page.goto('/tokens')
  await expect(page.locator('.app-header')).toContainText('Collecting usage')
  await page.getByRole('button', { name: 'Local status: Collecting usage' }).hover()
  await expect(page.getByRole('region', { name: 'Collector progress' })).toContainText(
    'codex: Collecting',
  )
  expect(progressRequests).toBe(1)
  await expect(page.locator('.app-header').getByText('Ready', { exact: true })).toHaveCount(0)
  await expect(
    page.getByText('No usage saved yet. Restart tokeninsights web to collect usage.'),
  ).toHaveCount(0)
  pending = 1
  stage = 'accepted'
  await expect(page.locator('.app-header')).toContainText('Processing usage', {
    timeout: 10_000,
  })
  await expect(
    page.getByText('No usage saved yet. Restart tokeninsights web to collect usage.'),
  ).toHaveCount(0)
  pending = 0
  published = true
  await expect(page.getByLabel('Total tokens: 647,000', { exact: true })).toBeVisible({
    timeout: 10_000,
  })
  await expect(page.locator('.app-header')).toContainText('Ready')
  expect(methods.every((method) => method === 'GET')).toBe(true)
})

test('token login and sign-out clear hosted usage', async ({ page }) => {
  let authenticated = false
  const tokens: string[] = []
  await page.route('**/api/v2/**', async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    if (path.endsWith('/auth/session')) {
      if (request.method() === 'POST') {
        const body: unknown = request.postDataJSON()
        if (
          typeof body === 'object' &&
          body !== null &&
          'token' in body &&
          typeof body.token === 'string'
        )
          tokens.push(body.token)
        authenticated = true
      } else authenticated = false
      return route.fulfill({ status: 204 })
    }
    if (!authenticated) return route.fulfill({ status: 401 })
    const body = path.endsWith('/instance')
      ? { ...mockBootstrap, serverKind: 'hosted' }
      : path.endsWith('/status')
        ? mockSyncStatus(1)
        : path.endsWith('/facets')
          ? mockFacets
          : mockDashboard('tokens')
    return route.fulfill({ json: body })
  })
  await page.goto('/tokens')
  await page.getByLabel('Access token', { exact: true }).fill('user-secret')
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 647,000', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Sign out', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Sign in to TokenInsights' })).toBeVisible()
  await expect(page.getByLabel('Total tokens: 647,000', { exact: true })).toHaveCount(0)
  expect(tokens).toEqual(['user-secret'])
  expect(await page.evaluate(() => Object.values(localStorage))).not.toContain('user-secret')
  expect(page.url()).not.toContain('user-secret')
})

for (const kind of ['personal']) {
  test(`${kind} refresh keeps header geometry stable and status details accessible`, async ({
    page,
  }) => {
    let releaseUsage: (() => void) | undefined
    const usageGate = new Promise<void>((resolve) => {
      releaseUsage = resolve
    })
    let holdUsage = false
    await page.route('**/api/v2/**', async (route) => {
      const path = new URL(route.request().url()).pathname
      if (holdUsage && path.endsWith('/usage')) {
        await usageGate
      }
      const body = path.endsWith('/instance')
        ? { ...mockBootstrap, serverKind: kind }
        : path.endsWith('/status')
          ? {
              ...mockSyncStatus(1),
              processUsage: { cpuPercent: 12.5, residentMemoryBytes: 64 * 1024 * 1024 },
            }
          : path.endsWith('/facets')
            ? mockFacets
            : mockDashboard('tokens')
      await route.fulfill({ json: body })
    })
    await page.goto('/tokens')
    const ready = page.getByRole('button', { name: 'Local status: Ready' })
    await expect(ready).toBeVisible()
    const before = await page
      .locator('.server-identity, .loading-status, .theme-button')
      .evaluateAll((elements) =>
        elements.map((element) => {
          const { x, y, width, height } = element.getBoundingClientRect()
          return { x, y, width, height }
        }),
      )
    holdUsage = true
    await page.getByRole('button', { name: 'Sort ascending', exact: true }).click()
    const refreshing = page.getByRole('button', { name: 'Local status: Refreshing usage…' })
    await expect(refreshing).toBeVisible()
    await expect(refreshing.locator('svg')).toHaveClass(/spin/)
    expect(
      await page
        .locator('.server-identity, .loading-status, .theme-button')
        .evaluateAll((elements) =>
          elements.map((element) => {
            const { x, y, width, height } = element.getBoundingClientRect()
            return { x, y, width, height }
          }),
        ),
    ).toEqual(before)
    await refreshing.hover()
    const details = page.getByRole('dialog', { name: 'Local status details' })
    await expect(details).toContainText('Refreshing usage…')
    await details.hover()
    await expect(details).toBeVisible()
    await page.getByRole('heading', { name: '/ Tokens', exact: true }).hover()
    await expect(details).toHaveCount(0)
    releaseUsage?.()
    await expect(ready).toBeVisible()
    await ready.focus()
    await expect(details).toContainText('Ready')
    await expect(details).toContainText('CPU 12.5%')
    await expect(details).toContainText('RAM 64 MiB')
    await page.keyboard.press('Escape')
    await expect(details).toHaveCount(0)
    await expect(ready).toBeFocused()
    await page.getByRole('button', { name: 'Theme: system. Change theme' }).focus()
    await page.setViewportSize({ width: 390, height: 844 })
    await ready.click()
    await expect(details).toBeVisible()
    const bounds = await details.boundingBox()
    if (!bounds) throw new Error('Missing status details')
    expect(bounds.x).toBeGreaterThanOrEqual(0)
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(390)
  })
}
