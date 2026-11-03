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
  protocolVersion: 2,
}

void test('URL handoffs attach the requested owner instead of configured default storage', () => {
  assert.deepEqual(backendArguments(), ['connect', '--json'])
  assert.deepEqual(backendArguments('http://127.0.0.1:43210/models?period=week'), [
    'connect',
    '--json',
    '--url',
    'http://127.0.0.1:43210',
  ])
  assert.throws(() => backendArguments('https://usage.example.com/'), /local dashboard/)
  assert.throws(() => backendArguments('http://user:secret@127.0.0.1:43210/'), /local dashboard/)
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
  for (const protocolVersion of [1, 3]) {
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
  assert.equal(
    launchURL('tokeninsights-electron://open?url=https%3A%2F%2Fexample.com'),
    'https://example.com/',
  )
})
