import { expect, test } from '@playwright/test'
import { dashboardSchema } from '../src/contracts'

const progress = (startedAt: number) => ({
  jobId: 99,
  startedAt,
  updatedAt: startedAt + 1000,
  lastSuccessfulAt: 0,
  totalSources: 2,
  checkedSources: 1,
  readySources: 1,
  failedSources: 0,
  discoveryComplete: true,
})

test('chart load failure preserves dashboard controls and results, and reload recovers', async ({
  page,
}) => {
  await page.route('**/assets/UsageChart-*.js', (route) => route.abort('failed'))
  await page.goto('/tokens')
  await expect(page.getByRole('alert')).toContainText('Chart couldn’t load')
  await expect(page.getByRole('navigation', { name: 'Analytics views' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Dashboard filters' })).toBeVisible()
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Tokens details', exact: true })).toBeVisible()
  await page.unroute('**/assets/UsageChart-*.js')
  await page.getByRole('button', { name: 'Reload page', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Usage over time', exact: true })).toBeVisible()
})

test('old status reads cannot overwrite a newly started sync', async ({ page }) => {
  let running = false
  let hold = false
  let captured = false
  let delivered = false
  let release: (() => void) | undefined
  await page.route('**/api/v1/sync', async (route) => {
    const post = route.request().method() === 'POST'
    if (post) running = true
    const snapshot = {
      phase: running ? 'syncing' : 'ready',
      running,
      error: '',
      revision: running ? 100 : 99,
      harnesses: {},
      progress: progress(running ? 2000 : 1000),
    }
    if (!post && hold && !captured) {
      captured = true
      await new Promise<void>((resolve) => {
        release = resolve
      })
      await route.fulfill({ json: snapshot })
      delivered = true
      return
    }
    await route.fulfill({ status: post ? 202 : 200, json: snapshot })
  })
  await page.goto('/tokens')
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  hold = true
  await page.getByRole('button', { name: 'Reload Data', exact: true }).click()
  await expect.poll(() => captured).toBe(true)
  await page.getByRole('button', { name: 'Sync Usage', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Syncing…', exact: true })).toBeDisabled()
  if (!release) throw new Error('Expected a held status request')
  release()
  await expect.poll(() => delivered).toBe(true)
  await expect(page.locator('.header-status')).toHaveText('Syncing…')
  await expect(page.getByRole('button', { name: 'Syncing…', exact: true })).toBeDisabled()
})

test('old empty days remain unconfirmed in table cells and expanded coverage', async ({ page }) => {
  await page.route('**/api/v1/sync', (route) =>
    route.fulfill({
      json: {
        phase: 'syncing',
        running: true,
        error: '',
        revision: 99,
        harnesses: {},
        progress: progress(2000),
      },
    }),
  )
  await page.route('**/api/v1/usage?*', async (route) => {
    const response = await route.fetch()
    const data = dashboardSchema.parse(await response.json())
    await route.fulfill({
      json: {
        ...data,
        coverage: [
          {
            day: '2026-09-20',
            status: 'empty',
            checkedAt: 1000,
            pendingSources: 0,
            failedSources: 0,
            hasUsage: false,
            total: 0,
          },
        ],
      },
    })
  })
  await page.goto('/tokens')
  const row = page
    .getByRole('region', { name: 'Scrollable results' })
    .getByRole('row', { name: /2026-09-20/ })
  await expect(row.locator('td.numeric')).toHaveText(['—', '—', '—', '—', '—', '—', '—'])
  await expect(row.getByRole('img')).toHaveCount(0)
  await page.getByText('Source coverage', { exact: true }).click()
  const coverage = page.getByRole('region', { name: 'Daily source coverage' })
  await expect(coverage).toContainText('Awaiting current check')
  await expect(coverage).not.toContainText('No usage found')
  await expect(coverage.locator('td.numeric')).toHaveText('—')
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
})

test('session search failure keeps its draft and selection and supports retry', async ({
  page,
}) => {
  let failing = true
  await page.route('**/api/v1/usage/facets?*', (route) => {
    if (failing && new URL(route.request().url()).searchParams.get('search') === '059')
      return route.fulfill({
        status: 500,
        json: { code: 'unavailable', message: 'Search unavailable' },
      })
    return route.continue()
  })
  await page.goto('/tokens?session=web-session-000')
  await page.getByRole('button', { name: 'Session 1', exact: true }).click()
  const search = page.getByRole('textbox', { name: 'Search session' })
  await search.fill('059')
  await expect(page.getByRole('alert')).toContainText('Session values couldn’t load')
  await expect(page.getByText('No matching values', { exact: true })).toHaveCount(0)
  await expect(search).toHaveValue('059')
  await expect(page.getByRole('button', { name: 'Remove session web-session-000' })).toBeVisible()
  failing = false
  await page.getByRole('button', { name: 'Retry session search' }).click()
  await expect(page.getByRole('checkbox', { name: 'web-session-059', exact: true })).toBeVisible()
  await expect(search).toHaveValue('059')
  await expect(page).toHaveURL(/session=web-session-000/)
})

test('sync and sorting preserve expanded Repo directories and focus', async ({ page }) => {
  let running = false
  await page.route('**/api/v1/sync', (route) => {
    if (route.request().method() === 'POST') running = true
    return route.fulfill({
      json: {
        phase: running ? 'syncing' : 'ready',
        running,
        error: '',
        revision: 99,
        harnesses: {},
        progress: progress(running ? 2000 : 1000),
      },
    })
  })
  await page.goto('/repo')
  await page.getByRole('button', { name: 'Show directories', exact: true }).click()
  const expanded = page.getByRole('button', { name: 'Hide directories', exact: true })
  await expect(expanded).toBeVisible()
  await page.getByRole('button', { name: 'Sync Usage', exact: true }).click()
  await expanded.focus()
  await expect(page.getByLabel('Sources checked')).toBeVisible()
  await expect(expanded).toBeFocused()
  await expect(page.getByRole('list', { name: 'Recorded directories' })).toBeVisible()
  await page
    .getByRole('region', { name: 'Repo details', exact: true })
    .getByRole('button', { name: 'Total', exact: true })
    .click()
  await expect(expanded).toBeVisible()
  await expect(page.getByRole('list', { name: 'Recorded directories' })).toBeVisible()
})

test('display preferences and filter drafts survive route loading and history', async ({
  page,
}) => {
  await page.goto('/tokens')
  const chart = page.getByRole('region', { name: 'Usage over time', exact: true })
  await chart.getByRole('button', { name: 'Input', exact: true }).click()
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  await page.getByRole('checkbox', { name: 'Output', exact: true }).uncheck()
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: 'Model', exact: true }).click()
  await page.getByRole('textbox', { name: 'Search model' }).fill('model-a')
  await page.keyboard.press('Escape')
  let release: (() => void) | undefined
  await page.route('**/api/v1/usage?*', async (route) => {
    if (new URL(route.request().url()).searchParams.get('tab') === 'models') {
      await new Promise<void>((resolve) => {
        release = resolve
      })
    }
    await route.continue()
  })
  await page.getByRole('link', { name: 'Models', exact: true }).click()
  await expect(page.getByRole('status', { name: 'Updating view results' })).toBeVisible()
  await page.getByRole('button', { name: 'Model', exact: true }).click()
  await expect(page.getByRole('textbox', { name: 'Search model' })).toHaveValue('model-a')
  await page.keyboard.press('Escape')
  await expect.poll(() => release !== undefined).toBe(true)
  if (!release) throw new Error('Expected a held view request')
  release()
  await expect(page.getByRole('region', { name: 'Models details', exact: true })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: 'Output', exact: true })).toHaveCount(0)
  await page.goBack()
  await expect(chart.getByRole('button', { name: 'Input', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  )
  await expect(page.getByRole('columnheader', { name: 'Output', exact: true })).toHaveCount(0)
})
