import { test } from 'node:test'
import assert from 'node:assert/strict'
import type { Bootstrap } from '@tokeninsights/contracts'
import { localLaunch } from '../src/launch.ts'

const instance: Bootstrap = {
  apiVersion: 'v2',
  serverKind: 'personal',
  permissions: ['read'],
  instanceId: 'local-instance',
  dataEpoch: 'local-data',
  datasetId: 'default',
  dataReadiness: 'ready',
  serverVersion: 'test',
  hostname: 'test',
  timezone: 'UTC',
  capabilities: ['usage', 'facets', 'web-dashboard'],
  defaults: {
    period: 'all',
    bucket: 'day',
    from: '',
    to: '',
    providers: [],
    models: [],
    harnesses: [],
    sessions: [],
  },
}

const personal: typeof fetch = async (input, options) => {
  assert(input instanceof URL)
  assert.equal(input.href, 'http://127.0.0.1:12345/api/v2/instance')
  assert.equal(options?.redirect, 'error')
  return new Response(JSON.stringify(instance))
}
const hosted: typeof fetch = async () =>
  new Response(JSON.stringify({ ...instance, serverKind: 'hosted' }))

void test('only a personal loopback instance opens the native viewer', async () => {
  assert.equal(await localLaunch('http://127.0.0.1:12345/tokens?period=week', personal), true)
  assert.equal(await localLaunch('http://127.0.0.1:12345/', hosted), false)
})

void test('hosted authentication, broken local probes and remote links stay in the browser', async () => {
  await Promise.all(
    [new Response(null, { status: 401 }), new Response('invalid json'), new Response('{}')].map(
      async (response) => {
        assert.equal(await localLaunch('http://127.0.0.1:12345/', async () => response), false)
      },
    ),
  )
  assert.equal(
    await localLaunch('http://127.0.0.1:12345/', async () => {
      throw new Error('offline')
    }),
    false,
  )
  assert.equal(
    await localLaunch('https://usage.example.com/', async () => {
      throw new Error('must not probe remote links')
    }),
    false,
  )
})
