// Runs only against the invocation-owned compose personal gateway service.
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { mkdtemp, rm } from 'node:fs/promises'
import { hostname, tmpdir } from 'node:os'
import { join } from 'node:path'
import { startDevChild } from './dev-lifecycle.ts'
import { workspacePath } from './workspace.ts'

const control = process.env.TOKENINSIGHTS_TEST_PERSONAL_CONTROL_URL
if (control === undefined) throw new Error('isolated personal gateway service required')
const requestTimeoutMs = 30_000
const request = (url: string, options: RequestInit = {}) =>
  fetch(url, { ...options, signal: AbortSignal.timeout(requestTimeoutMs) })
const response = await request(`${control}/connection`)
assert.equal(response.status, 200)
const descriptor: unknown = await response.json()
assert.ok(
  typeof descriptor === 'object' &&
    descriptor !== null &&
    'connection' in descriptor &&
    typeof descriptor.connection === 'string',
)
const connection = new URL(descriptor.connection)
const credentials = new URLSearchParams(connection.hash.slice(1))
const token = credentials.get('token')
const pin = credentials.get('pin')
assert.ok(token && pin)
const fixture = await mkdtemp(join(tmpdir(), 'tokeninsights-network-'))
const binary = workspacePath('packages/cli/bin/tokeninsights')
const env: NodeJS.ProcessEnv = { ...process.env }
for (const key of Object.keys(env))
  if (key.startsWith('TOKENINSIGHTS_') || key.startsWith('XDG_')) delete env[key]
Object.assign(env, {
  HOME: fixture,
  XDG_CONFIG_HOME: join(fixture, 'config'),
  XDG_DATA_HOME: join(fixture, 'data'),
  XDG_RUNTIME_DIR: join(fixture, 'run'),
  NO_COLOR: '1',
})
const args = (target: string) => [
  'web',
  '--connection',
  target,
  '--capture=false',
  '--open=false',
  '--port=0',
]
let output = ''
const viewer = startDevChild(
  binary,
  args(connection.href),
  { env },
  /Dashboard:\s+(http:\/\/127\.0\.0\.1:\d+)/,
  30_000,
  (chunk) => {
    output += chunk
  },
)
try {
  const dashboard = await viewer.ready
  const metadataResponse = await request(`${dashboard}/api/viewer/connection`)
  assert.equal(metadataResponse.status, 200)
  const metadata: unknown = await metadataResponse.json()
  assert.ok(typeof metadata === 'object' && metadata !== null)
  assert.ok('transport' in metadata && metadata.transport === 'tcp')
  assert.ok('hostname' in metadata && typeof metadata.hostname === 'string')
  assert.ok(metadata.hostname.length > 0 && metadata.hostname !== hostname())
  const serialized = JSON.stringify(metadata)
  assert.ok(!serialized.includes(token) && !serialized.includes(pin))
  assert.equal((await request(`${dashboard}/api/v2/status`)).status, 200)
  assert.equal(
    (
      await request(`${dashboard}/api/viewer/refresh`, {
        method: 'POST',
        headers: { Origin: dashboard, 'Content-Type': 'application/json' },
        body: '{}',
      })
    ).status,
    202,
  )
  // A valid format with a different key or secret must fail without local fallback.
  for (const [key, value] of [
    ['pin', '0'.repeat(64)],
    ['token', 'A'.repeat(43)],
  ]) {
    assert.ok(key && value)
    const changed = new URL(connection)
    const fragment = new URLSearchParams(credentials)
    fragment.set(key, value)
    changed.hash = fragment.toString()
    const rejected = spawnSync(binary, args(changed.href), {
      env,
      encoding: 'utf8',
      timeout: 30_000,
    })
    assert.notEqual(rejected.status, 0)
    assert.equal(rejected.error, undefined, 'connection failure must return before timeout')
    assert.ok(!(rejected.stdout + rejected.stderr).includes(token))
    if (key === 'token') assert.ok(!(rejected.stdout + rejected.stderr).includes(value))
  }
  assert.equal((await request(`${control}/stop`, { method: 'POST' })).status, 204)
  const expired = spawnSync(binary, args(connection.href), {
    env,
    encoding: 'utf8',
    timeout: 30_000,
  })
  assert.notEqual(expired.status, 0)
  assert.equal(expired.error, undefined)
  assert.ok(!(expired.stdout + expired.stderr).includes(token))
  assert.ok(!existsSync(join(fixture, 'data/tokeninsights')), 'TCP must not create local storage')
  assert.ok(!output.includes(token) && !output.includes(pin), 'viewer output exposes credentials')
  console.log('Verified isolated TCP query, Refresh, identity, bad pin/token and expired string.')
} finally {
  await viewer.stop()
  await rm(fixture, { recursive: true, force: true })
}
