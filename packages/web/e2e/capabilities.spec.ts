import { expect, test } from '@playwright/test'
import { mockBootstrap, mockDashboard, mockFacets, mockSyncStatus } from '../src/mocks/data'

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
          ? mockSyncStatus(1)
          : path.endsWith('/facets')
            ? mockFacets
            : mockDashboard('tokens')
      return route.fulfill({ json: body })
    })
    await page.goto('/tokens')
    await expect(page.getByLabel('Total tokens: 647,000', { exact: true })).toBeVisible()
    if (kind === 'personal') {
      await expect(page.getByRole('region', { name: 'Collector progress' })).toContainText(
        'Submitting usage',
      )
      expect(progressRequests.length).toBeGreaterThan(0)
    } else {
      await expect(page.getByRole('region', { name: 'Collector progress' })).toHaveCount(0)
      await page.getByRole('button', { name: 'Reload', exact: true }).click()
      await expect(page.getByRole('button', { name: 'Reload', exact: true })).toBeEnabled()
      expect(progressRequests).toHaveLength(0)
    }
  })
}

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
