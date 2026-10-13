import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { createHash, randomUUID } from 'node:crypto'
import { expect, test } from '@playwright/test'
import { z } from 'zod'
import {
  adminRequest,
  hostedFixturePointer,
  hostedOrigin,
  hostedBackend,
  hostedPassword,
  tokenSchema,
  userSchema,
} from './hosted-fixture'

const fixtureAccountSchema = userSchema.extend(tokenSchema.shape)

test.beforeEach(async ({ context }, testInfo) => {
  // Each test represents a separate client behind the trusted fixture proxy.
  // Preserve production admission limits without sharing the proxy's loopback quota.
  const hash = createHash('sha256').update(testInfo.testId).digest('hex')
  const address = `2001:db8:${hash.slice(0, 4)}:${hash.slice(4, 8)}:${hash.slice(8, 12)}::1`
  await context.setExtraHTTPHeaders({ 'X-Forwarded-For': address })
})

async function fixtureAccount(name: string) {
  const home = await readFile(hostedFixturePointer, 'utf8')
  return fixtureAccountSchema.parse(JSON.parse(await readFile(join(home, name + '.json'), 'utf8')))
}

async function operatorSocket() {
  const home = await readFile(hostedFixturePointer, 'utf8')
  return readFile(join(home, 'socket.txt'), 'utf8')
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

test('self-service API key revocation rejects the key and preserves independent browser sessions', async ({
  page,
}) => {
  await page.goto(hostedOrigin + '/tokens')
  await page.getByLabel('Username', { exact: true }).fill('alice')
  await page.getByLabel('Password', { exact: true }).fill(hostedPassword)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 120', { exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'Account settings', exact: true }).click()
  await page.getByLabel('Key name', { exact: true }).fill('Browser test laptop')
  await page.getByRole('button', { name: 'Create API key', exact: true }).click()
  const token = await page.getByLabel('API key', { exact: true }).inputValue()
  const headers = { Authorization: `Bearer ${token}` }
  expect((await fetch(hostedBackend + '/api/v2/usage', { headers })).status).toBe(200)
  await page.getByRole('button', { name: 'Done', exact: true }).click()
  await expect(page.getByLabel('API key', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Revoke Browser test laptop', exact: true }).click()
  await expect(
    page.getByRole('button', { name: 'Revoke Browser test laptop', exact: true }),
  ).toBeDisabled()
  expect((await fetch(hostedBackend + '/api/v2/usage', { headers })).status).toBe(401)
  await page.getByRole('link', { name: 'Return to usage', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 120', { exact: true })).toBeVisible()
  expect(page.url()).not.toContain(token)
  expect(await page.evaluate(() => Object.values(localStorage))).not.toContain(token)
})

test('password change signs out the browser, rejects old credentials and preserves API keys', async ({
  page,
}) => {
  const socket = await operatorSocket()
  const username = 'password-' + randomUUID().slice(0, 8)
  const user = await adminRequest(
    socket,
    { operation: 'create-user', displayName: username },
    userSchema,
  )
  await adminRequest(
    socket,
    { operation: 'reset-password', userId: user.userId, password: hostedPassword },
    z.object({}),
  )
  const key = await adminRequest(
    socket,
    { operation: 'create-token', userId: user.userId, permissions: ['read'] },
    tokenSchema,
  )
  await page.goto(hostedOrigin + '/settings')
  await page.getByLabel('Username', { exact: true }).fill(username)
  await page.getByLabel('Password', { exact: true }).fill(hostedPassword)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await page.getByRole('link', { name: 'Account settings', exact: true }).click()
  await page.getByLabel('Current password', { exact: true }).fill(hostedPassword)
  await page.getByLabel('New password', { exact: true }).fill('new hosted fixture password')
  await page.getByLabel('Confirm new password', { exact: true }).fill('new hosted fixture password')
  await page.getByRole('button', { name: 'Change password', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Sign in to TokenInsights' })).toBeVisible()
  await page.getByLabel('Username', { exact: true }).fill(username)
  await page.getByLabel('Password', { exact: true }).fill(hostedPassword)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Couldn’t sign in.')
  await page.getByLabel('Password', { exact: true }).fill('new hosted fixture password')
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByRole('link', { name: 'Account settings', exact: true })).toBeVisible()
  expect(
    (
      await fetch(hostedBackend + '/api/v2/usage', {
        headers: { Authorization: `Bearer ${key.token}` },
      })
    ).status,
  ).toBe(200)
})

test('enabled signup creates a browser session and an isolated account', async ({ page }) => {
  const username = 'signup-' + randomUUID().slice(0, 8)
  await page.goto(hostedOrigin + '/tokens')
  await page.getByRole('button', { name: 'Create an account', exact: true }).click()
  await page.getByLabel('Username', { exact: true }).fill(username)
  await page.getByLabel('Password', { exact: true }).fill(hostedPassword)
  await page.getByRole('button', { name: 'Create account', exact: true }).click()
  await page.getByRole('link', { name: 'Account settings', exact: true }).click()
  await expect(page.locator('.account-details')).toContainText(username)
  await expect(
    page.getByText('No API keys yet. Create one for your first collector.'),
  ).toBeVisible()
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
  await page.evaluate(() => document.getElementById('key-label')?.closest('form')?.requestSubmit())
  expect((await rejection).status()).toBe(409)
  await expect(page.getByRole('heading', { name: 'Sign in to TokenInsights' })).toBeVisible()
  await expect(page.locator('.account-details')).toHaveCount(0)
  await other.getByRole('link', { name: 'Account settings', exact: true }).click()
  await expect(other.locator('.account-details')).toContainText('bob')
  await expect(other.getByText('Old account must not create this', { exact: true })).toHaveCount(0)
})
