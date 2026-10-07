import { expect, test } from '@playwright/test'

test('estimated evidence stays separate through switching and browser history', async ({
  page,
}) => {
  let mutations = 0
  page.on('request', (request) => {
    if (request.method() === 'POST') mutations++
  })
  await page.goto('/models?period=all')
  await expect(page.getByLabel('Total tokens: 344,000', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Estimated', exact: true }).click()
  await expect(page).toHaveURL(/quality=estimated/)
  await expect(page.getByLabel('Total tokens: 120', { exact: true })).toBeVisible()
  await expect(page.getByText('Estimated; excluded from confirmed', { exact: true })).toBeVisible()
  await expect(page.getByLabel('Total tokens: 344,000', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Confirmed', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 344,000', { exact: true })).toBeVisible()
  await page.goBack()
  await expect(page.getByLabel('Total tokens: 120', { exact: true })).toBeVisible()
  expect(mutations).toBe(0)
})
