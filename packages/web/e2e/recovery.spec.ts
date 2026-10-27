import { expect, test } from './fixture'

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
