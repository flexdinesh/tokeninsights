import { expect, test } from '@playwright/test'

import { dashboardSchema } from '../src/contracts'

test('excluded usage stays separate through review, return and browser history', async ({
  page,
}) => {
  let mutations = 0
  page.on('request', (request) => {
    if (request.method() === 'POST') mutations++
  })
  await page.goto('/models?period=all')
  await expect(page.getByLabel('Total tokens: 344,000', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Confirmed', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Estimated', exact: true })).toHaveCount(0)
  await expect(page.locator('.excluded-usage-notice')).toContainText('120 tokens')
  await page.getByRole('button', { name: 'Review excluded usage', exact: true }).click()
  await expect(page).toHaveURL(/quality=estimated/)
  await expect(page.getByLabel('Excluded tokens: 120', { exact: true })).toBeVisible()
  await expect(page.getByText('Excluded from your usage total', { exact: true })).toBeVisible()
  await expect(
    page.getByRole('heading', { name: 'Excluded usage by model', exact: true }),
  ).toBeVisible()
  await expect(
    page.getByRole('heading', { name: 'Excluded models breakdown', exact: true }),
  ).toBeVisible()
  await expect(page.getByLabel('Total tokens: 344,000', { exact: true })).toHaveCount(0)
  await expect(
    page.getByRole('region', { name: 'Excluded usage results', exact: true }),
  ).toBeFocused()
  await page.getByRole('button', { name: 'Back to usage', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 344,000', { exact: true })).toBeVisible()
  await page.goBack()
  await expect(page.getByLabel('Excluded tokens: 120', { exact: true })).toBeVisible()
  expect(mutations).toBe(0)
})

test('excluded review retains filters and empty usage offers recovery', async ({ page }) => {
  await page.goto('/models?period=all&quality=estimated&model=estimated-model')
  await expect(page.getByLabel('Excluded tokens: 120', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Back to usage', exact: true }).click()
  await expect(page).toHaveURL(/model=estimated-model/)
  await expect(page.getByLabel('Total tokens: 0', { exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'No usage matches these filters' })).toBeVisible()
  await page.locator('.empty-state').getByRole('button', { name: 'Review excluded usage' }).click()
  await expect(page.getByLabel('Excluded tokens: 120', { exact: true })).toBeVisible()
  await expect(page).toHaveURL(/model=estimated-model/)
  await page.getByRole('button', { name: 'Back to usage', exact: true }).click()
  await page.locator('.empty-state').getByRole('button', { name: 'Clear filters' }).click()
  await expect(page.getByLabel('Total tokens: 344,000', { exact: true })).toBeVisible()
  expect(new URL(page.url()).searchParams.getAll('model')).toEqual([])
})

test('no matching excluded usage keeps the main dashboard free of review controls', async ({
  page,
}) => {
  await page.goto('/models?period=all&model=model-a')
  await expect(page.getByLabel('Total tokens: 172,000', { exact: true })).toBeVisible()
  await expect(page.locator('.excluded-usage-notice')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Review excluded usage' })).toHaveCount(0)
})

test('excluded disclosure respects Repo location filters', async ({ page }) => {
  const response = await page.request.get(
    '/api/v2/usage?tab=repo&locationGroup=directory&period=all',
  )
  const data = dashboardSchema.parse(await response.json())
  const directory = data.rows.find((row) => row.locationKey !== 'unknown')
  if (!directory) throw new Error('Expected a recorded fixture directory')
  await page.goto(
    `/repo?period=all&locationGroup=directory&directory=${encodeURIComponent(directory.locationKey)}`,
  )
  await expect(page.getByLabel('Total tokens: 68,800', { exact: true })).toBeVisible()
  await expect(page.locator('.excluded-usage-notice')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Review excluded usage' })).toHaveCount(0)
})

test('excluded summary failure can retry without hiding saved usage', async ({ page }) => {
  let failing = true
  await page.route('**/api/v2/usage?*', async (route) => {
    if (new URL(route.request().url()).searchParams.get('quality') === 'estimated' && failing) {
      await route.fulfill({ status: 503, json: { code: 'unavailable', message: 'Unavailable' } })
      return
    }
    await route.continue()
  })
  await page.goto('/models?period=all')
  await expect(page.getByLabel('Total tokens: 344,000', { exact: true })).toBeVisible()
  await expect(page.getByText('Excluded usage couldn’t load.')).toBeVisible({ timeout: 15_000 })
  await expect(page.getByLabel('Total tokens: 344,000', { exact: true })).toBeVisible()
  failing = false
  await page.getByRole('button', { name: 'Retry excluded usage' }).click()
  await expect(page.getByRole('button', { name: 'Review excluded usage' })).toBeVisible()
})

test('saved excluded evidence does not incorrectly suggest there is no saved usage', async ({
  page,
}) => {
  await page.route('**/api/v2/usage?*', async (route) => {
    if (new URL(route.request().url()).searchParams.get('quality') === 'estimated') {
      await route.continue()
      return
    }
    const response = await route.fetch()
    const data = dashboardSchema.parse(await response.json())
    await route.fulfill({
      json: {
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
      },
    })
  })
  await page.goto('/models?period=all')
  await expect(page.getByText('No usage included in your total yet.')).toBeVisible()
  await expect(page.getByText('No usage saved yet. Run tokeninsights sync.')).toHaveCount(0)
  await page.getByRole('button', { name: 'Review excluded usage' }).first().click()
  await expect(page.getByLabel('Excluded tokens: 120', { exact: true })).toBeVisible()
})

test('mobile review keeps the exclusion visible and supports keyboard return', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/tokens?period=all')
  const review = page.getByRole('button', { name: 'Review excluded usage' })
  await review.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByLabel('Excluded tokens: 120', { exact: true })).toBeVisible()
  await expect(page.getByText('Excluded from your usage total', { exact: true })).toBeVisible()
  await expect(
    page.getByRole('region', { name: 'Excluded usage results', exact: true }),
  ).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(page.getByRole('button', { name: 'Back to usage', exact: true })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page.getByLabel('Total tokens: 344,000', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
  await page.evaluate(() => {
    document.documentElement.style.fontSize = '200%'
  })
  await expect(page.getByRole('button', { name: 'Review excluded usage' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
})
