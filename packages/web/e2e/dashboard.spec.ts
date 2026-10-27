import { expect, test } from './fixture'

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
    .getByRole('region', { name: 'Scrollable results', exact: true })
    .evaluate((element) => element.scrollWidth > element.clientWidth)
  expect(scrollsHorizontally).toBe(true)
  await resize.focus()
  await page.keyboard.press('ArrowRight')
  await expect(resize).toHaveAttribute('aria-valuenow', '144')
})

test('saved dashboard filters survive reload and history; session pagination uses real data', async ({
  page,
}) => {
  await page.goto('/tokens')
  await expect(page.getByLabel('Total tokens: 258,000', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Model', exact: true }).click()
  await page.getByRole('checkbox', { name: 'model-a', exact: true }).check()
  await page.getByRole('button', { name: 'Done', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 129,000', { exact: true })).toBeVisible()
  await expect(page.getByText(/^Sessions \d+ shown \/ \d+ synced$/)).toHaveText(
    'Sessions 30 shown / 80 synced',
  )
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
  await expect(
    page.getByRole('region', { name: 'Sessions details', exact: true }).locator('tbody tr'),
  ).toHaveCount(50)
  await page.getByRole('button', { name: 'Next page' }).click()
  await expect(
    page.getByRole('region', { name: 'Sessions details', exact: true }).locator('tbody tr'),
  ).toHaveCount(10)
  await expect(
    page.getByRole('region', { name: 'Sessions details', exact: true }).locator('footer'),
  ).toContainText('60 rows')
  await expect(page.getByText(/^Sessions \d+ shown \/ \d+ synced$/)).toHaveText(
    'Sessions 60 shown / 80 synced',
  )
  await expect(
    page.getByRole('region', { name: 'Sessions details', exact: true }).locator('footer'),
  ).toContainText('258K')
  await page.setViewportSize({ width: 1440, height: 400 })
  const total = page
    .getByRole('region', { name: 'Sessions details', exact: true })
    .getByRole('button', { name: 'Total', exact: true })
  await total.scrollIntoViewIfNeeded()
  const scroll = await page.evaluate(() => scrollY)
  expect(scroll).toBeGreaterThan(0)
  await total.click()
  await expect(page).toHaveURL(/sort=total/)
  await expect(page.getByRole('region', { name: 'Sessions details', exact: true })).toBeVisible()
  expect(await page.evaluate(() => scrollY)).toBe(scroll)
})

test('mobile keyboard filters and enlarged text remain reachable without page overflow', async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/tokens')
  const harness = page.getByRole('button', { name: 'Harness', exact: true })
  await harness.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('textbox', { name: 'Search harness' })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(harness).toBeFocused()
  const sessions = page
    .getByRole('navigation', { name: 'Analytics views' })
    .getByRole('link', { name: 'Sessions', exact: true })
  await sessions.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('region', { name: 'Sessions details', exact: true })).toBeVisible()
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
    .toBe(true)
  await page.evaluate(() => {
    document.documentElement.style.fontSize = '200%'
  })
  await expect(harness).toBeVisible()
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
    .toBe(true)
})
