import { test } from 'node:test'
import assert from 'node:assert/strict'
import { connectTarget } from '../src/launch.ts'

const launch = 'tokeninsights-electron://launch?id=0123456789abcdef0123456789abcdef'

void test('each existing-app handoff connects its exact native launch request', async () => {
  const calls: Array<string[] | undefined> = []
  await Promise.all(
    [undefined, launch].map(async (link) => {
      const connection = await connectTarget(
        'backend',
        () => {},
        link,
        async (_executable, _onExit, args) => {
          calls.push(args)
          return {
            descriptor: {
              url: 'http://127.0.0.1:12345/',
              instanceId: 'instance',
              protocolVersion: 3,
            },
            close() {},
            acknowledge() {},
          }
        },
      )
      assert.equal(connection.descriptor.instanceId, 'instance')
    }),
  )
  assert.deepEqual(calls, [
    ['connect', '--json'],
    ['connect', '--json', '--launch', launch],
  ])
})

void test('failed handoffs and standalone launches remain errors without fallback', async () => {
  await Promise.all(
    [undefined, launch].map(async (link) => {
      let calls = 0
      await assert.rejects(
        connectTarget(
          'backend',
          () => {},
          link,
          async () => {
            calls += 1
            throw new Error('requested connection failed')
          },
        ),
        /requested connection failed/,
      )
      assert.equal(calls, 1)
    }),
  )
})
