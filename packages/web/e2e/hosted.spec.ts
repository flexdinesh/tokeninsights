import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { expect, test } from '@playwright/test'
import { z } from 'zod'
import {
  adminRequest,
  hostedFixturePointer,
  hostedOrigin,
  tokenSchema,
  userSchema,
} from './hosted-fixture'

const fixtureAccountSchema = userSchema.extend(tokenSchema.shape)

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
  await page.getByLabel('Access token', { exact: true }).fill(alice.token)
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
  await page.getByLabel('Access token', { exact: true }).fill(bob.token)
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

test('real token revocation invalidates browser sessions and removes visible cached totals', async ({
  page,
}) => {
  const alice = await fixtureAccount('alice')
  const socket = await operatorSocket()
  // Each test owns its token; revocation cannot affect the user-switch fixture.
  const token = await adminRequest(
    socket,
    { operation: 'create-token', userId: alice.userId, permissions: ['read'] },
    tokenSchema,
  )
  await page.goto(hostedOrigin + '/tokens')
  await page.getByLabel('Access token', { exact: true }).fill(token.token)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByLabel('Total tokens: 120', { exact: true })).toBeVisible()
  // Observe the polling boundary before asserting UI recovery; the default
  // assertion deadline is the same length as the status polling interval.
  const rejectedStatus = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === '/api/v2/status' && response.status() === 401,
  )
  await adminRequest(socket, { operation: 'revoke-token', tokenId: token.tokenId }, z.object({}))
  // Revocation is observed through the same status polling used without Reload.
  await rejectedStatus
  await expect(page.getByRole('heading', { name: 'Sign in to TokenInsights' })).toBeVisible()
  await expect(page.getByLabel('Total tokens: 120', { exact: true })).toHaveCount(0)
})
