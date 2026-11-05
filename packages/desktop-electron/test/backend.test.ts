import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { watch } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { backendArguments, connectBackend } from '../src/backend.ts'
import { externalURL, launchURL, localNavigation } from '../src/links.ts'
import { localBackendDescriptorSchema } from '@tokeninsights/contracts/local'

const descriptor = {
  url: 'http://127.0.0.1:12345/',
  instanceId: 'test-instance',
  protocolVersion: 3,
}

const launch = 'tokeninsights-electron://launch?id=0123456789abcdef0123456789abcdef'
void test('handoffs contain an opaque rendezvous ID and never a target or secret', () => {
  assert.deepEqual(backendArguments(), ['connect', '--json'])
  assert.deepEqual(backendArguments(launch), ['connect', '--json', '--launch', launch])
  for (const value of [
    'http://127.0.0.1:1234/',
    'ssh://host',
    'tokeninsights-electron://launch?id=bad&token=secret',
  ]) {
    assert.throws(() => backendArguments(value), /Invalid desktop launch link/)
  }
})

void test('backend descriptor rejects remote origins, credentials and incompatible protocols', () => {
  assert.deepEqual(localBackendDescriptorSchema.parse(descriptor), descriptor)
  for (const url of [
    'https://example.com/',
    'http://127.0.0.1.evil.test:12345/',
    'file:///tmp/index.html',
    'http://user:secret@127.0.0.1:12345/',
    'http://127.0.0.1:12345/?command=run',
  ]) {
    assert.equal(localBackendDescriptorSchema.safeParse({ ...descriptor, url }).success, false)
  }
  for (const protocolVersion of [1, 2, 4]) {
    assert.equal(
      localBackendDescriptorSchema.safeParse({ ...descriptor, protocolVersion }).success,
      false,
    )
  }
})

void test('connector keeps stdin open until the window closes', { timeout: 5_000 }, async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tokeninsights-electron-'))
  const marker = join(directory, 'closed')
  let unexpectedExit = false
  try {
    const script = `import { writeFileSync } from 'node:fs'; process.stdin.resume(); process.stdin.on('end', () => { writeFileSync(${JSON.stringify(marker)}, 'closed'); }); console.log(${JSON.stringify(JSON.stringify(descriptor))})`
    const connection = await connectBackend(
      process.execPath,
      () => {
        unexpectedExit = true
      },
      ['--input-type=module', '--eval', script],
    )
    assert.deepEqual(connection.descriptor, descriptor)
    await assert.rejects(readFile(marker), { code: 'ENOENT' })
    const changed = Promise.withResolvers<void>()
    const watcher = watch(directory, () => changed.resolve())
    try {
      connection.close()
      await changed.promise
      assert.equal(await readFile(marker, 'utf8'), 'closed')
      assert.equal(unexpectedExit, false)
    } finally {
      watcher.close()
      connection.close()
    }
  } finally {
    await rm(directory, { recursive: true, force: true })
  }
})

void test('connector rejects malformed startup output and early exits', async () => {
  await assert.rejects(
    connectBackend(process.execPath, () => {}, ['--eval', 'console.log("not json")']),
    /incompatible/,
  )
  await assert.rejects(
    connectBackend(process.execPath, () => {}, ['--eval', 'process.exit(1)']),
    /before readiness/,
  )
})

void test(
  'desktop acknowledges a live connector once, while preserving its lease',
  { timeout: 5_000 },
  async () => {
    const directory = await mkdtemp(join(tmpdir(), 'tokeninsights-electron-ack-'))
    const marker = join(directory, 'ack')
    const received = Promise.withResolvers<void>()
    const watcher = watch(directory, (_event, filename) => {
      if (filename === 'ack') received.resolve()
    })
    try {
      const script = `import { writeFileSync } from 'node:fs'; process.stdin.on('data', (data) => writeFileSync(${JSON.stringify(marker)}, data)); console.log(${JSON.stringify(JSON.stringify(descriptor))})`
      const connection = await connectBackend(
        process.execPath,
        () => assert.fail('acknowledgement closed the connector'),
        ['--input-type=module', '--eval', script],
      )
      try {
        await assert.rejects(readFile(marker), { code: 'ENOENT' })
        connection.acknowledge()
        connection.acknowledge()
        await received.promise
        assert.equal(await readFile(marker, 'utf8'), 'ready\n')
      } finally {
        connection.close()
      }
    } finally {
      watcher.close()
      await rm(directory, { recursive: true, force: true })
    }
  },
)

void test('unexpected connector exit notifies the window owner', async () => {
  const exited = Promise.withResolvers<void>()
  const connection = await connectBackend(process.execPath, () => exited.resolve(), [
    '--eval',
    `console.log(${JSON.stringify(JSON.stringify(descriptor))})`,
  ])
  await exited.promise
  connection.close()
})

void test('links stay within the backend origin or open HTTP(S) in the browser', () => {
  assert.equal(localNavigation('http://127.0.0.1:12345/tokens', descriptor.url), true)
  for (const url of [
    'http://127.0.0.1:12346/',
    'https://example.com/',
    'http://127.0.0.1.evil.test:12345/',
    'javascript:alert(1)',
    'http://user:secret@127.0.0.1:12345/',
  ]) {
    assert.equal(localNavigation(url, descriptor.url), false)
  }
  assert.equal(externalURL('file:///etc/passwd'), undefined)
  assert.equal(externalURL('javascript:alert(1)'), undefined)
  assert.equal(externalURL('https://user:secret@example.com/'), undefined)
  assert.equal(launchURL('tokeninsights://open?url=https://example.com'), undefined)
  assert.equal(
    launchURL('tokeninsights-electron://open?url=https://example.com&url=https://evil.test'),
    undefined,
  )
  assert.equal(launchURL('tokeninsights-electron://user@open?url=https://example.com'), undefined)
  assert.equal(launchURL(launch), launch)
})
