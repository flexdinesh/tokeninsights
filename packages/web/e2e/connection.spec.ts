import { test, expect } from './fixture'

// Capture and publication are asynchronous; status/progress poll every five
// seconds. Allow the same visibility window as finite local maintenance.
const captureVisibilityTimeoutMs = 30_000

test('Refresh requests capture once while filters and page reload query saved data', async ({
  page,
}) => {
  const refreshes: string[] = []
  page.on('request', (request) => {
    if (new URL(request.url()).pathname === '/api/viewer/refresh') refreshes.push(request.method())
  })
  await page.goto('/tokens?period=all')
  const connection = page.getByRole('group', { name: 'Backend connection' })
  await expect(connection).toContainText('LOCAL')
  await expect(connection).toContainText('connected')
  await expect(page.getByLabel('Total tokens: 344,000')).toBeVisible()
  await page.getByRole('link', { name: 'Models', exact: true }).click()
  await page.reload()
  await expect(page.getByLabel('Total tokens: 344,000')).toBeVisible()
  expect(refreshes).toEqual([])
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Refresh', exact: true })).toBeEnabled({
    timeout: captureVisibilityTimeoutMs,
  })
  await expect(page.getByRole('button', { name: 'Backend status: Ready' })).toBeVisible({
    timeout: captureVisibilityTimeoutMs,
  })
  expect(refreshes).toEqual(['POST'])
  await expect(page.getByLabel('Total tokens: 344,000')).toBeVisible()
})

test('connection failure keeps the requested target and saved results', async ({ page }) => {
  await page.goto('/tokens?period=all')
  await expect(page.getByLabel('Total tokens: 344,000')).toBeVisible()
  const requested = await page.getByLabel('Connection state').textContent()
  await page.route('**/api/v2/status', (route) => route.abort('failed'))
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(page.getByLabel('Connection state')).toContainText('unavailable')
  expect(requested).toBeTruthy()
  await expect(page.getByLabel('Connection state')).toContainText('LOCAL')
  await expect(page.getByLabel('Total tokens: 344,000')).toBeVisible()
})
