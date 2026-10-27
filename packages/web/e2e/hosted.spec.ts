import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { createHash } from 'node:crypto'
import { expect, test, fixtureMetadata } from './fixture'
import { tokenSchema, userSchema } from './hosted-fixture'

let hostedOrigin = ''
let hostedPassword = ''

const fixtureAccountSchema = userSchema.extend(tokenSchema.shape)

test.beforeEach(async ({ context }, testInfo) => {
  ;({ hostedOrigin, hostedPassword } = fixtureMetadata())
  // Each test represents a separate client behind the trusted fixture proxy.
  // Preserve production admission limits without sharing the proxy's loopback quota.
  const hash = createHash('sha256').update(testInfo.testId).digest('hex')
  const address = `2001:db8:${hash.slice(0, 4)}:${hash.slice(4, 8)}:${hash.slice(8, 12)}::1`
  await context.setExtraHTTPHeaders({ 'X-Forwarded-For': address })
})

async function fixtureAccount(name: string) {
  const home = fixtureMetadata().hostedHome
  return fixtureAccountSchema.parse(JSON.parse(await readFile(join(home, name + '.json'), 'utf8')))
}

test('real hosted sessions isolate users sharing one database and never poll collector progress', async ({
  page,
  context,
}) => {
  const alice = await fixtureAccount('alice')
  const bob = await fixtureAccount('bob')
  const requests: string[] = []
  page.on('request', (request) => {
    requests.push(new URL(request.url()).pathname)
  })
  await page.goto(hostedOrigin + '/tokens?session=old-account-session')
  await page.getByLabel('Username', { exact: true }).fill('alice')
  await page.getByLabel('Password', { exact: true }).fill(hostedPassword)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 120', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Reload', exact: true })).toHaveCount(0)
  expect(page.url()).not.toContain('old-account-session')
  const cookies = await context.cookies(hostedOrigin)
  const session = cookies.find((cookie) => cookie.name === 'tokeninsights_session')
  expect(session?.httpOnly).toBe(true)
  expect(session?.secure).toBe(true)
  expect(session?.sameSite).toBe('Lax')
  const ingestionAccess = await page.request.get(hostedOrigin + '/api/v3/ingestion/capabilities')
  expect(ingestionAccess.status()).toBe(403)
  await page.getByRole('button', { name: 'Sign out', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Sign in to TokenInsights' })).toBeVisible()
  await expect(page.getByLabel('Total tokens: 120', { exact: true })).toHaveCount(0)
  await page.getByLabel('Username', { exact: true }).fill('bob')
  await page.getByLabel('Password', { exact: true }).fill(hostedPassword)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 420', { exact: true })).toBeVisible()
  await expect(page.getByLabel('Total tokens: 120', { exact: true })).toHaveCount(0)
  expect(requests.some((path) => path.includes('collector-progress'))).toBe(false)
  expect(page.url()).not.toContain(alice.token)
  expect(page.url()).not.toContain(bob.token)
  const savedPreferences = await page.evaluate(() => Object.values(localStorage))
  expect(savedPreferences).not.toContain(alice.token)
  expect(savedPreferences).not.toContain(bob.token)
})

test('a shared-cookie account switch rejects an old settings mutation before touching the new account', async ({
  page,
  context,
}) => {
  await page.goto(hostedOrigin + '/tokens')
  await page.getByLabel('Username', { exact: true }).fill('alice')
  await page.getByLabel('Password', { exact: true }).fill(hostedPassword)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await page.getByRole('link', { name: 'Account settings', exact: true }).click()
  await page.getByLabel('Key name', { exact: true }).fill('Old account must not create this')
  const other = await context.newPage()
  await other.goto(hostedOrigin + '/tokens')
  await other.getByRole('button', { name: 'Sign out', exact: true }).click()
  await other.getByLabel('Username', { exact: true }).fill('bob')
  await other.getByLabel('Password', { exact: true }).fill(hostedPassword)
  await other.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(other.getByLabel('Total tokens: 420', { exact: true })).toBeVisible()
  const rejection = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === '/api/v2/account/keys' &&
      response.request().method() === 'POST',
  )
  // Submit without focusing the old tab: verify mutation binding independently
  // of the normal account-query refresh on focus.
  await page
    .getByLabel('Key name', { exact: true })
    .evaluate((input) => input.closest('form')?.requestSubmit())
  expect((await rejection).status()).toBe(409)
  await expect(page.getByRole('heading', { name: 'Sign in to TokenInsights' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Account', exact: true })).toHaveCount(0)
  await other.getByRole('link', { name: 'Account settings', exact: true }).click()
  await expect(other.getByRole('region', { name: 'Account', exact: true })).toContainText('bob')
  await expect(other.getByText('Old account must not create this', { exact: true })).toHaveCount(0)
})
