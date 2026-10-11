import { expect, test } from '@playwright/test'
import { InstanceResponseV2, UsageResponseV2 } from '../src/generated/api'
import { dashboardSchema } from '../src/contracts'

test.describe('server reporting timezone', () => {
  test.use({ timezoneId: 'America/Los_Angeles' })

  test('UTC server dates stay UTC in a non-UTC browser', async ({ page }) => {
    const instant = Date.parse('2026-01-01T00:30:00Z')
    await page.route('**/api/v2/instance', async (route) => {
      const response = await route.fetch()
      const instance = InstanceResponseV2.parse(await response.json())
      await route.fulfill({ json: { ...instance, timezone: 'UTC' } })
    })
    await page.route('**/api/v2/usage?*', async (route) => {
      const response = await route.fetch()
      const data = dashboardSchema.parse(await response.json())
      const row = data.rows[0]
      if (!row) throw new Error('Expected a saved session fixture')
      await route.fulfill({
        json: { ...data, rows: [{ ...row, date: instant }], lastSynced: instant },
      })
    })
    await page.goto('/sessions?period=all')
    await expect(page.locator('.table-panel time')).toHaveText('Jan 1, 2026')
    await expect(page.locator('.app-header time')).toHaveText('Last ingestion Jan 1, 2026, 00:30')
    await expect(page.locator('.app-footer')).toContainText('UTC')
    expect(await page.evaluate(() => Intl.DateTimeFormat().resolvedOptions().timeZone)).toBe(
      'America/Los_Angeles',
    )
  })
})

test('saved ingestion shows available rows without source completeness markers', async ({
  page,
}) => {
  await page.goto('/tokens')
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  await expect(page.locator('.coverage-indicator')).toHaveCount(0)
  await expect(page.getByText('Source coverage', { exact: true })).toHaveCount(0)
  await expect(page.getByLabel('Sources checked')).toHaveCount(0)
  await expect(page.getByText('Last ingestion', { exact: false })).toBeVisible()
})

test('empty server explains manual collection without a browser mutation', async ({ page }) => {
  await page.route('**/api/v2/usage?*', async (route) => {
    const response = await route.fetch()
    const data = dashboardSchema.parse(await response.json())
    await route.fulfill({
      json: {
        ...data,
        rows: [],
        chart: [],
        rowCount: 0,
        lastSynced: 0,
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
  const mutations: string[] = []
  page.on('request', (request) => {
    if (new URL(request.url()).pathname.startsWith('/api/') && request.method() !== 'GET') {
      mutations.push(request.method())
    }
  })
  await page.goto('/tokens')
  await expect(page.getByText('No usage saved yet. Run tokeninsights sync.')).toBeVisible()
  await page.getByRole('button', { name: 'Reload', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Reload', exact: true })).toBeEnabled()
  expect(mutations).toEqual([])
})

test('dimension charts show filtered token shares on bars, labels, and tooltips', async ({
  page,
}) => {
  for (const { tab, title, shares } of [
    { tab: 'models', title: 'Usage by model', shares: ['50%', '50%'] },
    { tab: 'providers', title: 'Usage by provider', shares: ['50%', '50%'] },
    { tab: 'harnesses', title: 'Usage by harness', shares: ['100%'] },
  ]) {
    await page.goto(`/${tab}`)
    const chart = page.getByRole('region', { name: title, exact: true })
    await expect(chart.locator('.recharts-label-list text')).toHaveText(shares)
    await expect(chart.locator('.chart-share')).toHaveText(shares)
    await chart.locator('.recharts-bar-rectangle path').first().hover()
    await expect(chart.locator('.recharts-tooltip-wrapper')).toContainText(`(${shares[0] ?? ''})`)
  }

  await page.goto('/models')
  const chart = page.getByRole('region', { name: 'Usage by model', exact: true })
  await chart.getByRole('button', { name: 'model-a 50%', exact: true }).click()
  await expect(chart.locator('.recharts-label-list text')).toHaveText(['100%'])
  await expect(chart.getByRole('button', { name: 'model-a 100%', exact: true })).toBeVisible()

  await page.setViewportSize({ width: 390, height: 844 })
  await expect(chart.locator('.recharts-label-list text')).toBeVisible()
  await expect(chart.getByRole('button', { name: 'model-a 100%', exact: true })).toBeVisible()

  await page.route('**/api/v2/usage?*', async (route) => {
    const response = await route.fetch()
    const data = UsageResponseV2.parse(await response.json())
    const row = data.chart[0]
    if (!row) throw new Error('Expected model chart fixture')
    const displayedGroups = 12
    const displayedTotal = data.summary.total / 2
    data.chart = Array.from({ length: displayedGroups }, (_, index) => ({
      ...row,
      key: `model-${index}`,
      name: `organization/model-family-${index}-extended-thinking`,
      total: displayedTotal / displayedGroups,
    }))
    await route.fulfill({ json: data })
  })
  await page.goto('/models')
  const expectedShares = Array.from({ length: 12 }, () => '4.2%')
  await expect(chart.locator('.chart-share')).toHaveText(expectedShares)
  await expect(chart.locator('.recharts-label-list text')).toHaveText(expectedShares)
  expect(
    await chart.locator('.chart-canvas').evaluate((el) => el.scrollWidth > el.clientWidth),
  ).toBe(true)
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)

  await page.evaluate(() => {
    document.documentElement.style.fontSize = '200%'
  })
  const sharesFitButtons = await chart.locator('.chart-filter').evaluateAll((buttons) =>
    buttons.every((button) => {
      const share = button.querySelector('.chart-share')
      const name = button.querySelector('.chart-filter-name')
      if (!share || !name) return false
      const bounds = button.getBoundingClientRect()
      const shareBounds = share.getBoundingClientRect()
      return (
        shareBounds.left >= bounds.left &&
        shareBounds.right <= bounds.right &&
        name.scrollWidth <= name.clientWidth
      )
    }),
  )
  expect(sharesFitButtons).toBe(true)
  expect(
    await chart.evaluate((element) => {
      const bounds = element.getBoundingClientRect()
      return bounds.left >= 0 && bounds.right <= innerWidth
    }),
  ).toBe(true)
})

test('repo charts show full filtered token shares in both location groupings', async ({
  page,
}, testInfo) => {
  await page.goto('/repo')
  const repositoryChart = page.getByRole('region', { name: 'Usage by repository', exact: true })
  await expect(repositoryChart.locator('.recharts-label-list text')).toHaveText(['100%'])
  await repositoryChart.locator('.recharts-bar-rectangle path').first().hover()
  await expect(repositoryChart.locator('.recharts-tooltip-wrapper')).toContainText('(100%)')

  await page.getByRole('combobox', { name: 'Group by location' }).click()
  await page.getByRole('option', { name: 'Directory' }).click()
  const directoryChart = page.getByRole('region', { name: 'Usage by directory', exact: true })
  const directoryShares = Array.from({ length: 5 }, () => '20%')
  await expect(directoryChart.locator('.recharts-label-list text')).toHaveText(directoryShares)
  await directoryChart.locator('.recharts-bar-rectangle path').first().hover()
  await expect(directoryChart.locator('.recharts-tooltip-wrapper')).toContainText('(20%)')
  await page.mouse.move(0, 0)
  await page.screenshot({ path: testInfo.outputPath('repo-desktop.png'), fullPage: true })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.screenshot({ path: testInfo.outputPath('repo-mobile.png'), fullPage: true })

  await page.route('**/api/v2/usage?*', async (route) => {
    const response = await route.fetch()
    const data = UsageResponseV2.parse(await response.json())
    const row = data.chart[0]
    if (!row) throw new Error('Expected repo chart fixture')
    const displayedGroups = 12
    const displayedTotal = data.summary.total / 2
    data.chart = Array.from({ length: displayedGroups }, (_, index) => ({
      ...row,
      key: `repo-${index}`,
      name: `organization/project-${index}`,
      total: displayedTotal / displayedGroups,
    }))
    await route.fulfill({ json: data })
  })
  await page.goto('/repo')
  await expect(repositoryChart.locator('.recharts-label-list text')).toHaveText(
    Array.from({ length: 12 }, () => '4.2%'),
  )
  await page.evaluate(() => {
    document.documentElement.style.fontSize = '200%'
  })
  expect(
    await repositoryChart
      .locator('.chart-canvas')
      .evaluate((el) => el.scrollWidth > el.clientWidth),
  ).toBe(true)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('table columns resize by drag and keyboard without sorting', async ({ page }) => {
  await page.goto('/tokens')
  const details = page.getByRole('region', { name: 'Tokens details', exact: true })
  const header = details.getByRole('columnheader', { name: 'Input' })
  const resize = details.getByRole('separator', { name: 'Resize Input column' })
  await expect(resize).toHaveAttribute('aria-valuenow', '128')

  await resize.focus()
  await page.keyboard.press('ArrowRight')
  await expect(resize).toHaveAttribute('aria-valuenow', '144')
  await page.keyboard.press('Home')
  await expect(resize).toHaveAttribute('aria-valuenow', '128')

  const before = await header.boundingBox()
  const handle = await resize.boundingBox()
  expect(before).not.toBeNull()
  expect(handle).not.toBeNull()
  if (!before || !handle) return
  const startX = handle.x + handle.width / 2
  const y = handle.y + handle.height / 2
  await page.mouse.move(startX, y)
  await page.mouse.down()
  await page.mouse.move(startX + 80, y, { steps: 4 })
  await page.mouse.up()

  await expect(resize).toHaveAttribute('aria-valuenow', '208')
  const after = await header.boundingBox()
  expect(after?.width).toBeGreaterThan(before.width + 70)
  await expect(header).toHaveAttribute('aria-sort', 'none')
  await resize.dblclick()
  await expect(resize).toHaveAttribute('aria-valuenow', '128')

  await page.setViewportSize({ width: 390, height: 844 })
  const scrollsHorizontally = await details
    .locator('.table-scroll')
    .evaluate((element) => element.scrollWidth > element.clientWidth)
  expect(scrollsHorizontally).toBe(true)
  await resize.focus()
  await page.keyboard.press('ArrowRight')
  await expect(resize).toHaveAttribute('aria-valuenow', '144')
})

test('saved usage, seven views, filtering, history, pagination, and read-only Reload', async ({
  page,
}) => {
  const errors: string[] = []
  let syncRequests = 0
  page.on('request', (request) => {
    if (new URL(request.url()).pathname === '/api/v2/status' && request.method() === 'POST')
      syncRequests++
  })
  page.on('pageerror', (error) => errors.push(error.message))
  await page.goto('/')
  await expect(page.getByRole('region', { name: 'Filtered usage summary' })).toBeVisible()
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Usage over time', exact: true })).toBeVisible()
  await expect(page.locator('.recharts-surface')).toBeVisible()
  await expect(page.getByRole('button', { name: 'This month', exact: true })).toBeVisible()
  for (const tab of ['Models', 'Providers', 'Harnesses', 'Sessions', 'Context', 'Repo', 'Tokens']) {
    await page
      .getByRole('navigation', { name: 'Analytics views' })
      .getByRole('link', { name: tab, exact: true })
      .click()
    await expect(page).toHaveURL(new RegExp(`/${tab.toLowerCase()}\\?`))
    await expect(page.getByRole('region', { name: `${tab} details`, exact: true })).toBeVisible()
    await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
    await expect(page.locator('.session-coverage')).toHaveText('Sessions 60 shown / 80 synced')
    await expect(page.getByLabel('Sessions shown: 60', { exact: true })).toBeVisible()
    await expect(page.locator('.sessions-card')).toContainText(
      '80 synced across all dates & harnesses',
    )
    if (tab === 'Harnesses') {
      const details = page.getByRole('region', { name: 'Harnesses details', exact: true })
      await expect(
        details.locator('tbody tr').first().locator('.identity-detail > span'),
      ).toHaveText(['anthropic', 'openai', 'model-a', 'model-b'])
    }
    if (tab === 'Repo') {
      const details = page.getByRole('region', { name: 'Repo details', exact: true })
      await expect(page.getByRole('combobox', { name: 'Group by location' })).toBeVisible()
      await expect(page.getByRole('combobox', { name: 'Breakdown by' })).toHaveCount(0)
      await page.getByRole('combobox', { name: 'Group by location' }).click()
      await expect(page.getByRole('option', { name: 'Repository' })).toBeVisible()
      await expect(page.getByRole('option', { name: 'Directory' })).toBeVisible()
      await expect(page.getByRole('option', { name: 'Worktree' })).toHaveCount(0)
      await expect(page.getByRole('option', { name: 'Branch' })).toHaveCount(0)
      await page.keyboard.press('Escape')
      for (const column of ['Providers', 'Harnesses', 'Models']) {
        await expect(details.getByRole('columnheader', { name: column, exact: true })).toBeVisible()
      }
      const totalRow = details.locator('tbody tr').first()
      const providers = totalRow.locator('td').nth(1).locator('.dimension-values > span')
      await expect(providers).toHaveText(['anthropic', 'openai'])
      await expect(totalRow.locator('td').nth(2).locator('.dimension-values > span')).toHaveText([
        'pi',
      ])
      await expect(totalRow.locator('td').nth(3).locator('.dimension-values > span')).toHaveText([
        'model-a',
        'model-b',
      ])
      const providerLines = await providers.evaluateAll((items) =>
        items.map((item) => item.getBoundingClientRect().top),
      )
      expect(providerLines[1]).toBeGreaterThan(providerLines[0] ?? 0)
      await expect(totalRow.getByText('project-0')).toBeHidden()
      await expect(totalRow.getByText('Some usage has no recorded directory')).toHaveCount(0)
      await totalRow.getByRole('button', { name: 'Show directories' }).click()
      await expect(totalRow.getByText('project-0')).toBeVisible()
      await expect(totalRow.getByText('project-1')).toBeVisible()
      await expect(totalRow.getByText('project-2')).toBeVisible()
      await expect(totalRow.getByText('project-3')).toBeVisible()
      await expect(totalRow.getByText('Some usage has no recorded directory')).toBeVisible()
      await page.getByRole('combobox', { name: 'Group by location' }).click()
      await page.getByRole('option', { name: 'Directory' }).click()
      const unknownDirectory = details.locator('.identity-cell').filter({
        has: page.getByText('unknown', { exact: true }),
      })
      await expect(unknownDirectory).toContainText('No recorded directory')
      await expect(unknownDirectory.getByRole('button', { name: 'Show directories' })).toHaveCount(
        0,
      )
    }
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
    .getByRole('link', { name: 'Sessions', exact: true })
    .click()
  await expect(page.locator('.table-panel tbody tr')).toHaveCount(50)
  await page.getByRole('button', { name: 'Next page' }).click()
  await expect(page.locator('.table-panel tbody tr')).toHaveCount(10)
  await expect(page.locator('.results-summary')).toContainText('60 rows')
  await expect(page.locator('.session-coverage')).toHaveText('Sessions 60 shown / 80 synced')
  await expect(page.locator('.results-summary')).toContainText('258K')
  expect(syncRequests).toBe(0)
  await page.getByRole('button', { name: 'Reload', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Reload', exact: true })).toBeEnabled()
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Reload Data', exact: true })).toHaveCount(0)
  await page.reload()
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  expect(syncRequests).toBe(0)
  expect(errors).toEqual([])
  await page.getByRole('button', { name: 'All time', exact: true }).click()
  await expect(page.locator('.session-coverage')).toHaveText('Sessions 80 shown / 80 synced')
  await expect(page.getByLabel('Sessions shown: 80', { exact: true })).toBeVisible()
})

test('path routes keep shared dashboard stable while view data loads', async ({ page }) => {
  await page.goto('/tokens')
  await expect(page.getByRole('region', { name: 'Filtered usage summary' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Dashboard filters' })).toBeVisible()
  const tokensLink = page.getByRole('link', { name: 'Tokens', exact: true })
  await expect(tokensLink).toHaveAttribute('aria-current', 'page')

  const navigation = page.getByRole('navigation', { name: 'Analytics views' })
  await navigation.evaluate((element) => {
    element.dataset.mounted = 'routes'
  })
  await page.getByRole('region', { name: 'Filtered usage summary' }).evaluate((element) => {
    element.dataset.mounted = 'summary'
  })
  await page.route('**/api/v2/usage?*', async (route) => {
    if (new URL(route.request().url()).searchParams.get('tab') === 'models') {
      await new Promise((resolve) => setTimeout(resolve, 500))
    }
    await route.continue()
  })

  await page.getByRole('link', { name: 'Models', exact: true }).click()
  await expect(page).toHaveURL(/\/models\?/)
  await expect(page.getByRole('status', { name: 'Updating view results' })).toBeVisible()
  await expect(navigation).toHaveAttribute('data-mounted', 'routes')
  await expect(navigation).toBeVisible()
  await expect(
    page.locator('[aria-label="Filtered usage summary"][data-mounted="summary"]'),
  ).toBeVisible()
  await expect(page.getByRole('region', { name: 'Models details', exact: true })).toBeVisible()
  const modelBars = page
    .getByRole('region', { name: 'Usage by model', exact: true })
    .locator('.recharts-bar-rectangle path')
  await expect(modelBars).toHaveCount(2)
  expect(
    new Set(await modelBars.evaluateAll((bars) => bars.map((bar) => bar.getAttribute('fill'))))
      .size,
  ).toBe(2)

  await page.getByRole('link', { name: 'Providers', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Providers details', exact: true })).toBeVisible()
  const providerBars = page
    .getByRole('region', { name: 'Usage by provider', exact: true })
    .locator('.recharts-bar-rectangle path')
  await expect(providerBars).toHaveCount(2)
  expect(
    new Set(await providerBars.evaluateAll((bars) => bars.map((bar) => bar.getAttribute('fill'))))
      .size,
  ).toBe(2)

  await page.goto('/providers?period=all')
  await expect(page).toHaveURL(/\/providers\?[^#]*period=all/)
  await expect(page.getByRole('region', { name: 'Providers details', exact: true })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('region', { name: 'Providers details', exact: true })).toBeVisible()
})

test('static summaries preserve totals while chart controls switch the timeline', async ({
  page,
}) => {
  await page.goto('/tokens')
  const total = page.getByLabel('Total tokens: 258,000', { exact: true })
  await expect(total).toBeVisible()
  const chart = page.getByRole('region', { name: 'Usage over time', exact: true })
  const summary = page.getByRole('region', { name: 'Filtered usage summary' })
  await expect(summary.getByRole('button')).toHaveCount(0)
  await chart.getByRole('button', { name: 'Input', exact: true }).click()
  await expect(chart.getByRole('button', { name: 'Input', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  )
  await expect(total).toBeVisible()
  await total.click()
  await expect(chart.getByRole('button', { name: 'Input', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  )
  await chart.getByRole('button', { name: 'Total', exact: true }).click()
  await expect(chart.getByRole('button', { name: 'Total', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  )
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByRole('button', { name: 'Harness', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Harness', exact: true }).click()
  await expect(page.getByRole('textbox', { name: 'Search harness' })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: 'Harness', exact: true })).toBeFocused()
})

test('sorting preserves page scroll position', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 400 })
  await page.goto('/sessions')
  await expect(page.getByRole('region', { name: 'Usage over time', exact: true })).toBeVisible()
  const details = page.getByRole('region', { name: 'Sessions details', exact: true })
  await expect(details).toBeVisible()
  const totalHeader = details.getByRole('button', { name: 'Total', exact: true })
  await totalHeader.scrollIntoViewIfNeeded()
  const before = await page.evaluate(() => window.scrollY)
  expect(before).toBeGreaterThan(0)
  await page.route('**/api/v2/usage?*', async (route) => {
    if (new URL(route.request().url()).searchParams.get('sort') === 'total') {
      await new Promise((resolve) => setTimeout(resolve, 500))
    }
    await route.continue()
  })

  await totalHeader.click()
  await expect(page).toHaveURL(/sort=total/)
  await expect(page.getByRole('status', { name: 'Updating view results' })).toBeAttached()
  expect(await page.evaluate(() => window.scrollY)).toBe(before)
  await expect(details).toBeVisible()
  expect(await page.evaluate(() => window.scrollY)).toBe(before)
})

test('themes, keyboard filters, mobile layout, and scalable typography', async ({
  page,
}, testInfo) => {
  const fontRequests: string[] = []
  page.on('request', (request) => {
    if (request.resourceType() === 'font') fontRequests.push(request.url())
  })
  await page.emulateMedia({ colorScheme: 'light' })
  await page.goto('/')
  await expect(page.getByRole('region', { name: 'Filtered usage summary' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Usage over time', exact: true })).toBeVisible()
  await expect(page.locator('.recharts-surface')).toBeVisible()
  const bodySize = await page
    .locator('body')
    .evaluate((element) => Number.parseFloat(getComputedStyle(element).fontSize))
  expect(bodySize).toBeGreaterThanOrEqual(14)
  const heading = page.getByRole('heading', { name: 'Token usage', exact: true })
  await expect(heading).toBeVisible()
  const sidebar = await page.locator('.dashboard-sidebar').boundingBox()
  const content = await page.locator('.dashboard-main').boundingBox()
  if (!sidebar || !content) throw new Error('Desktop navigation and dashboard must be visible')
  expect(sidebar.x + sidebar.width).toBeLessThanOrEqual(content.x)
  expect(
    await page.locator('body').evaluate(async (element) => {
      await document.fonts.ready
      return {
        family: getComputedStyle(element).fontFamily,
        loaded: [...document.fonts].some(
          (font) => font.family.includes('DM Sans') && font.status === 'loaded',
        ),
      }
    }),
  ).toMatchObject({ family: expect.stringContaining('DM Sans'), loaded: true })
  expect(fontRequests.length).toBeGreaterThan(0)
  expect(fontRequests.every((url) => new URL(url).origin === new URL(page.url()).origin)).toBe(true)

  async function expectReadableTheme(theme: 'light' | 'dark') {
    const colors = await page.locator('body').evaluate((element) => {
      const style = getComputedStyle(element)
      const canvas = document.createElement('canvas')
      canvas.width = 1
      canvas.height = 1
      const context = canvas.getContext('2d')
      if (!context) throw new Error('Color contrast measurement needs a canvas')
      function measure(color: string) {
        if (!context) throw new Error('Color contrast measurement needs a canvas')
        context.fillStyle = color
        context.fillRect(0, 0, 1, 1)
        const [red, green, blue] = context.getImageData(0, 0, 1, 1).data
        if (red === undefined || green === undefined || blue === undefined)
          throw new Error('Expected opaque theme colors')
        const channels = [red, green, blue]
        const [r, g, b] = channels.map((value) => {
          const channel = value / 255
          return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
        })
        if (r === undefined || g === undefined || b === undefined)
          throw new Error('Expected three color channels')
        return {
          luminance: 0.2126 * r + 0.7152 * g + 0.0722 * b,
          channelSpread: Math.max(...channels) - Math.min(...channels),
        }
      }
      return { background: measure(style.backgroundColor), text: measure(style.color) }
    })
    const lighter = Math.max(colors.background.luminance, colors.text.luminance)
    const darker = Math.min(colors.background.luminance, colors.text.luminance)
    const minimumTextContrast = 4.5
    const neutralChannelTolerance = 12
    expect((lighter + 0.05) / (darker + 0.05)).toBeGreaterThanOrEqual(minimumTextContrast)
    expect(colors.background.channelSpread).toBeLessThanOrEqual(neutralChannelTolerance)
    if (theme === 'light')
      expect(colors.background.luminance).toBeGreaterThan(colors.text.luminance)
    else expect(colors.background.luminance).toBeLessThan(colors.text.luminance)
  }

  async function expectNavigationReachable() {
    const navigation = page.getByRole('navigation', { name: 'Analytics views' })
    for (const name of [
      'Tokens',
      'Models',
      'Providers',
      'Harnesses',
      'Sessions',
      'Context',
      'Repo',
    ]) {
      const link = navigation.getByRole('link', { name, exact: true })
      await link.scrollIntoViewIfNeeded()
      await expect(link).toBeVisible()
      await link.focus()
      await expect(link).toBeFocused()
      const bounds = await link.boundingBox()
      if (!bounds) throw new Error(`${name} navigation must remain reachable`)
      expect(bounds.x).toBeGreaterThanOrEqual(0)
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(
        await page.evaluate(() => window.innerWidth),
      )
    }
  }

  await expectReadableTheme('light')
  await page.emulateMedia({ colorScheme: 'dark' })
  await expectReadableTheme('dark')
  await page.emulateMedia({ colorScheme: 'light' })
  expect(
    await page
      .getByLabel('Total tokens: 258,000', { exact: true })
      .evaluate((element) => Number.parseFloat(getComputedStyle(element).fontSize)),
  ).toBeGreaterThan(bodySize)
  const toolbarControls = [
    page.getByRole('button', { name: 'Harness', exact: true }),
    page.getByRole('button', { name: 'This month', exact: true }),
    page.getByRole('combobox', { name: 'Time bucket', exact: true }),
  ]
  const controlHeights = await Promise.all(
    toolbarControls.map(async (control) => (await control.boundingBox())?.height),
  )
  await expect(page.getByRole('button', { name: 'Restore CLI defaults', exact: true })).toHaveCount(
    0,
  )
  for (const height of controlHeights) expect(height).toBeGreaterThanOrEqual(24)
  await page.getByRole('button', { name: 'Theme: system. Change theme' }).click()
  await expectReadableTheme('light')
  await page.getByRole('button', { name: 'Theme: light. Change theme' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expectReadableTheme('dark')
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expect(page.locator('.recharts-surface')).toBeVisible()
  await page.getByRole('button', { name: 'Harness', exact: true }).focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('textbox', { name: 'Search harness' })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: 'Harness', exact: true })).toBeFocused()
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByRole('button', { name: 'Reload', exact: true })).toBeVisible()
  await expectNavigationReachable()
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth))
    .toBe(true)
  await page.screenshot({ path: testInfo.outputPath('mobile.png'), fullPage: true })
  await page.setViewportSize({ width: 1440, height: 1100 })
  await page.evaluate(() => {
    document.documentElement.style.fontSize = '200%'
  })
  for (const readout of await page.locator('.summary-card strong').all()) {
    const value = await readout.boundingBox()
    const container = await readout.locator('..').boundingBox()
    if (!value || !container) throw new Error('Usage readout must remain visible at 200% text')
    expect(value.x).toBeGreaterThanOrEqual(container.x)
    expect(value.x + value.width).toBeLessThanOrEqual(container.x + container.width)
  }
  for (const period of await page.locator('tbody .identity-name').all()) {
    await expect(period).toBeVisible()
    expect(await period.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    )
  }
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth))
    .toBe(true)
  await expect(page.getByRole('button', { name: 'Reload', exact: true })).toBeVisible()
  await expectNavigationReachable()
  await page.evaluate(() => {
    document.documentElement.style.fontSize = ''
  })
  await page.screenshot({ path: testInfo.outputPath('desktop-dark.png'), fullPage: true })
  await page.getByRole('button', { name: 'Theme: dark. Change theme' }).click()
  await page.getByRole('button', { name: 'Theme: system. Change theme' }).click()
  await expectReadableTheme('light')
  await page.screenshot({ path: testInfo.outputPath('desktop-light.png'), fullPage: true })
})

for (const address of ['127.0.0.1', 'localhost']) {
  test(`page server via ${address} owns every API request`, async ({ page }) => {
    const origin = `http://${address}:18765`
    const instanceResponse = await page.request.get(`${origin}/api/v2/instance`)
    const instance = InstanceResponseV2.parse(await instanceResponse.json())
    const requests: string[] = []
    page.on('request', (request) => {
      if (new URL(request.url()).pathname.startsWith('/api/')) requests.push(request.url())
    })
    await page.goto(`${origin}/tokens`)
    await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
    await expect(page.locator('.server-identity')).toHaveText(instance.hostname)
    await expect(page.getByRole('button', { name: 'Choose data source' })).toHaveCount(0)
    await page.getByRole('button', { name: 'Session', exact: true }).click()
    await page.getByRole('textbox', { name: 'Search session' }).fill('059')
    await expect(page.getByRole('checkbox', { name: 'web-session-059', exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Done', exact: true }).click()
    const sync = page.waitForRequest(
      (request) => request.url() === `${origin}/api/v2/status` && request.method() === 'GET',
    )
    await page.getByRole('button', { name: 'Reload', exact: true }).click()
    await sync
    await expect(page.getByRole('button', { name: 'Reload', exact: true })).toBeEnabled()
    await page.reload()
    await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
    expect(requests.every((url) => new URL(url).origin === origin)).toBe(true)
    expect(requests.some((url) => new URL(url).pathname === '/api/v2/usage')).toBe(true)
    expect(requests.some((url) => new URL(url).pathname === '/api/v2/usage/facets')).toBe(true)
  })
}

test('custom dates, session search, and saved usage after status failure', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('region', { name: 'Filtered usage summary' })).toBeVisible()
  await page.getByRole('button', { name: 'This month', exact: true }).click()
  await page.getByLabel('From date', { exact: true }).fill('2099-01-01')
  await page.getByRole('button', { name: 'Apply range' }).click()
  await expect(page.getByRole('heading', { name: 'No usage matches these filters' })).toBeVisible()
  await expect(page.getByLabel('Total tokens: 0', { exact: true })).toBeVisible()
  await expect(page.locator('.session-coverage')).toHaveText('Sessions 0 shown / 80 synced')
  await page.getByRole('button', { name: 'Clear All', exact: true }).click()
  await page.getByRole('button', { name: 'Session', exact: true }).click()
  await page.getByRole('textbox', { name: 'Search session' }).fill('059')
  await page.getByRole('checkbox', { name: 'web-session-059', exact: true }).check()
  await page.getByRole('button', { name: 'Done', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 4,300', { exact: true })).toBeVisible()
  await expect(page.locator('.session-coverage')).toHaveText('Sessions 1 shown / 80 synced')
  await page.route('**/api/v2/status', (route) =>
    route.fulfill({ status: 503, json: { code: 'unavailable', message: 'Status unavailable' } }),
  )
  await page.getByRole('button', { name: 'Reload', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Retry Request', exact: true })).toBeVisible()
  await expect(page.getByLabel('Total tokens: 4,300', { exact: true })).toBeVisible()
})
