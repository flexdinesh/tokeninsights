import { expect, test } from '@playwright/test'

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

test('Reload cancels an older status read and never submits collection', async ({ page }) => {
  let revision = 99
  let hold = false
  let captured = false
  let release: (() => void) | undefined
  const methods: string[] = []
  await page.route('**/api/v1/sync', async (route) => {
    methods.push(route.request().method())
    const snapshot = { phase: 'ready', running: false, error: '', revision, harnesses: {} }
    if (hold && !captured) {
      captured = true
      await new Promise<void>((resolve) => {
        release = resolve
      })
    }
    await route.fulfill({ json: snapshot })
  })
  await page.goto('/tokens')
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  hold = true
  await expect.poll(() => captured, { timeout: 10000 }).toBe(true)
  revision = 100
  // Status polling does not disable Reload; it can supersede an older read.
  await page.getByRole('button', { name: 'Reload', exact: true }).click()
  if (!release) throw new Error('Expected a held status request')
  release()
  await expect(page.getByRole('button', { name: 'Reload', exact: true })).toBeEnabled()
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  expect(methods.every((method) => method === 'GET')).toBe(true)
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

test('Reload and sorting preserve expanded Repo directories and focus', async ({ page }) => {
  await page.goto('/repo')
  await page.getByRole('button', { name: 'Show directories', exact: true }).click()
  const expanded = page.getByRole('button', { name: 'Hide directories', exact: true })
  await expect(expanded).toBeVisible()
  await page.getByRole('button', { name: 'Reload', exact: true }).click()
  await expanded.focus()
  await expect(page.getByRole('button', { name: 'Reload', exact: true })).toBeEnabled()
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
