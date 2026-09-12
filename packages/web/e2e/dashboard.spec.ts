import { expect, test } from '@playwright/test'
import { InstanceResponse } from '../src/generated/api'

const localSource = 'http://127.0.0.1:18765'
const remoteSource = 'http://127.0.0.1:18766'

test('startup sync, six views, filtering, history, pagination, and refresh', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  await page.goto('/')
  await expect(page.getByRole('region', { name: 'Filtered usage summary' })).toBeVisible()
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Usage over time', exact: true })).toBeVisible()
  await expect(page.locator('.recharts-surface')).toBeVisible()
  await expect(page.getByRole('button', { name: 'This week', exact: true })).toBeVisible()
  for (const tab of ['Models', 'Providers', 'Harnesses', 'Sessions', 'Context', 'Tokens']) {
    await page
      .getByRole('navigation', { name: 'Analytics views' })
      .getByRole('button', { name: tab, exact: true })
      .click()
    await expect(page.getByRole('region', { name: `${tab} details`, exact: true })).toBeVisible()
    await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
    await expect(page.locator('.session-coverage')).toHaveText('Sessions 60 shown / 80 synced')
    await expect(page.getByLabel('Sessions shown: 60', { exact: true })).toBeVisible()
    await expect(page.locator('.sessions-card')).toContainText(
      '80 synced across all dates & harnesses',
    )
  }
  await page.getByRole('button', { name: 'Model', exact: true }).click()
  await page.getByRole('checkbox', { name: 'model-a', exact: true }).check()
  await page.getByRole('button', { name: 'Done', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 129,000', { exact: true })).toBeVisible()
  await expect(page.locator('.session-coverage')).toHaveText('Sessions 30 shown / 80 synced')
  await expect(page).toHaveURL(/model=model-a/)
  await page.reload()
  await expect(page.getByLabel('Total tokens: 129,000', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Remove model model-a' }).click()
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  await page.goBack()
  await expect(page.getByLabel('Total tokens: 129,000', { exact: true })).toBeVisible()
  await page.goForward()
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  await page
    .getByRole('navigation', { name: 'Analytics views' })
    .getByRole('button', { name: 'Sessions', exact: true })
    .click()
  await expect(page.locator('tbody tr')).toHaveCount(50)
  await page.getByRole('button', { name: 'Next page' }).click()
  await expect(page.locator('tbody tr')).toHaveCount(10)
  await expect(page.locator('.results-summary')).toContainText('60 rows')
  await expect(page.locator('.session-coverage')).toHaveText('Sessions 60 shown / 80 synced')
  await expect(page.locator('.results-summary')).toContainText('258K')
  await page.getByRole('button', { name: 'Sync Usage', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Sync Usage', exact: true })).toBeEnabled()
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Reload Data', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  expect(errors).toEqual([])
  await page.getByRole('button', { name: 'All time', exact: true }).click()
  await expect(page.locator('.session-coverage')).toHaveText('Sessions 80 shown / 80 synced')
  await expect(page.getByLabel('Sessions shown: 80', { exact: true })).toBeVisible()
})

test('themes, keyboard filters, mobile layout, and scalable typography', async ({
  page,
}, testInfo) => {
  await page.goto('/')
  await expect(page.getByRole('region', { name: 'Filtered usage summary' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Usage over time', exact: true })).toBeVisible()
  await expect(page.locator('.recharts-surface')).toBeVisible()
  expect(await page.locator('body').evaluate((element) => getComputedStyle(element).fontSize)).toBe(
    '15px',
  )
  expect(
    await page
      .getByRole('heading', { name: 'Usage', exact: true })
      .evaluate((element) => getComputedStyle(element).fontSize),
  ).toBe('38px')
  expect(
    await page
      .getByLabel('Total tokens: 258,000', { exact: true })
      .evaluate((element) => getComputedStyle(element).fontSize),
  ).toBe('30px')
  const toolbarControls = [
    page.getByRole('button', { name: 'Harness', exact: true }),
    page.getByRole('button', { name: 'This week', exact: true }),
    page.getByRole('button', { name: 'Restore CLI defaults', exact: true }),
    page.getByRole('combobox', { name: 'Time bucket', exact: true }),
  ]
  const controlHeights = await Promise.all(
    toolbarControls.map(async (control) => (await control.boundingBox())?.height),
  )
  expect(controlHeights).toEqual([32, 32, 32, 32])
  await page.getByRole('button', { name: 'Theme: system. Change theme' }).click()
  await page.getByRole('button', { name: 'Theme: light. Change theme' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expect(page.locator('.recharts-surface')).toBeVisible()
  await page.getByRole('button', { name: 'Harness', exact: true }).focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('textbox', { name: 'Search harness' })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: 'Harness', exact: true })).toBeFocused()
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByRole('button', { name: 'Sync Usage', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  )
  await page.screenshot({ path: testInfo.outputPath('mobile.png'), fullPage: true })
  await page.setViewportSize({ width: 1440, height: 1100 })
  await page.evaluate(() => {
    document.documentElement.style.fontSize = '200%'
  })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  )
  await expect(page.getByRole('button', { name: 'Sync Usage', exact: true })).toBeVisible()
  await page.evaluate(() => {
    document.documentElement.style.fontSize = ''
  })
  await page.getByRole('heading', { name: 'Usage', exact: true }).click()
  await page.screenshot({ path: testInfo.outputPath('desktop-dark.png'), fullPage: true })
  await page.getByRole('button', { name: 'Theme: dark. Change theme' }).click()
  await page.getByRole('button', { name: 'Theme: system. Change theme' }).click()
  await page.screenshot({ path: testInfo.outputPath('desktop-light.png'), fullPage: true })
})

test('remote source switching, persistence, sync, and later failure', async ({ page }) => {
  const instanceResponse = await page.request.get(`${remoteSource}/api/v1/instance`)
  expect(instanceResponse.ok()).toBe(true)
  const remoteInstance = InstanceResponse.parse(await instanceResponse.json())

  await page.goto('/')
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Model', exact: true }).click()
  await page.getByRole('checkbox', { name: 'model-a', exact: true }).check()
  await page.getByRole('button', { name: 'Done', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 129,000', { exact: true })).toBeVisible()

  await page.getByRole('button', { name: 'Choose data source' }).click()
  await page.getByLabel('Add source').fill('127.0.0.1:18766')
  await page.getByRole('button', { name: 'Add', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Choose data source' })).toContainText(
    remoteInstance.hostname,
  )
  await expect(page.getByRole('button', { name: 'Choose data source' })).toContainText(remoteSource)
  await expect(page).toHaveURL(/model=model-a/)
  await expect(page.getByLabel('Total tokens: 777', { exact: true })).toBeVisible()

  await page.reload()
  await expect(page.getByRole('button', { name: 'Choose data source' })).toContainText(remoteSource)
  await expect(page).toHaveURL(/model=model-a/)
  await expect(page.getByLabel('Total tokens: 777', { exact: true })).toBeVisible()

  const remoteSync = page.waitForRequest(
    (request) => request.url() === `${remoteSource}/api/v1/sync` && request.method() === 'POST',
  )
  await page.getByRole('button', { name: 'Sync Usage', exact: true }).click()
  await remoteSync
  await expect(page.getByRole('button', { name: 'Sync Usage', exact: true })).toBeEnabled()
  await expect(page.getByLabel('Total tokens: 777', { exact: true })).toBeVisible()

  await page.getByRole('button', { name: 'Choose data source' }).click()
  await page
    .getByRole('listitem')
    .filter({ hasText: localSource })
    .getByRole('button', { name: /Select .* source/ })
    .click()
  await expect(page).toHaveURL(/model=model-a/)
  await expect(page.getByLabel('Total tokens: 129,000', { exact: true })).toBeVisible()

  await page.getByRole('button', { name: 'Choose data source' }).click()
  await page
    .getByRole('listitem')
    .filter({ hasText: remoteSource })
    .getByRole('button', { name: /Select .* source/ })
    .click()
  await expect(page.getByLabel('Total tokens: 777', { exact: true })).toBeVisible()

  await page.route(`${remoteSource}/api/v1/**`, (route) => route.abort('connectionfailed'))
  await page.reload()
  await expect(page.getByRole('button', { name: 'Choose data source' })).toContainText(remoteSource)
  await expect(page.getByRole('heading', { name: /Couldn.t connect to/ })).toBeVisible()
  await page.getByRole('button', { name: 'Choose data source' }).click()
  await expect(page.getByRole('listitem').filter({ hasText: remoteSource })).toBeVisible()
})

test('custom dates, session search, and explicit inspection after sync failure', async ({
  page,
}) => {
  await page.goto('/')
  await expect(page.getByRole('region', { name: 'Filtered usage summary' })).toBeVisible()
  await page.getByRole('button', { name: 'This week', exact: true }).click()
  await page.getByLabel('From date', { exact: true }).fill('2099-01-01')
  await page.getByRole('button', { name: 'Apply range' }).click()
  await expect(page.getByRole('heading', { name: 'No matching usage' })).toBeVisible()
  await expect(page.getByLabel('Total tokens: 0', { exact: true })).toBeVisible()
  await expect(page.locator('.session-coverage')).toHaveText('Sessions 0 shown / 80 synced')
  await page.getByRole('button', { name: 'Clear All', exact: true }).click()
  await page.getByRole('button', { name: 'Session', exact: true }).click()
  await page.getByRole('textbox', { name: 'Search session' }).fill('059')
  await page.getByRole('checkbox', { name: 'web-session-059', exact: true }).check()
  await page.getByRole('button', { name: 'Done', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 4,300', { exact: true })).toBeVisible()
  await expect(page.locator('.session-coverage')).toHaveText('Sessions 1 shown / 80 synced')
  await page.route('**/api/v1/sync', (route) =>
    route.fulfill({
      json: {
        running: false,
        phase: 'failed',
        harnesses: { pi: 'failed' },
        error: 'Sync failed. See terminal details, retry, or inspect existing data.',
        revision: 99,
      },
    }),
  )
  await page.reload()
  await expect(
    page.getByRole('button', { name: 'Inspect Existing Data', exact: true }),
  ).toBeVisible()
  await expect(page.getByRole('region', { name: 'Filtered usage summary' })).not.toBeVisible()
  await page.getByRole('button', { name: 'Inspect Existing Data', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 4,300', { exact: true })).toBeVisible()
})
