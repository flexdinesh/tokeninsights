import { expect, test } from './fixture'

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
