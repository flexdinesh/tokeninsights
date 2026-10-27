import assert from 'node:assert/strict'
import { createServer } from 'node:net'
import { createRequire } from 'node:module'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { parseDevOptions, runDevSession, startDevChild } from '../src/dev-lifecycle.ts'
import type { DevChild } from '../src/dev-lifecycle.ts'
import { workspaceRoot } from '../src/workspace.ts'

const quiet = () => {}
const readyPattern = /Ready: (http:\/\/127\.0\.0\.1:\d+)/

function server(port = 0, ignoreTerm = false): DevChild {
  return startDevChild(
    process.execPath,
    [
      '--input-type=module',
      '-e',
      `
    import { createServer } from 'node:http';
    const server = createServer((_, response) => response.end('ready'));
    server.listen(${port}, '127.0.0.1', () => {
      console.log('Ready: http://127.0.0.1:' + server.address().port);
    });
    process.on('SIGTERM', () => { ${ignoreTerm ? '' : 'server.close();'} });
  `,
    ],
    {},
    readyPattern,
    5000,
    quiet,
  )
}

async function assertReleased(url: string) {
  const listener = createServer()
  await new Promise<void>((resolve, reject) => {
    listener.once('error', reject)
    listener.listen(Number(new URL(url).port), '127.0.0.1', resolve)
  })
  await new Promise<void>((resolve, reject) =>
    listener.close((error) => (error ? reject(error) : resolve())),
  )
}

void test('development port options default to automatic and reject invalid input', () => {
  assert.deepEqual(parseDevOptions([], 'combined'), { apiPort: 0, webPort: 0 })
  assert.deepEqual(parseDevOptions(['--api-port', '0', '--web-port', '65535'], 'combined'), {
    apiPort: 0,
    webPort: 65535,
  })
  for (const value of ['-1', '65536', '3.5', 'abc', '']) {
    assert.throws(() => parseDevOptions(['--api-port', value], 'server'))
  }
  assert.throws(() => parseDevOptions(['--web-port', '1234'], 'server'))
  assert.throws(() => parseDevOptions(['--api-port'], 'combined'))
  assert.throws(() => parseDevOptions(['--unknown', '1'], 'combined'))
})

void test('automatic listeners coexist; an occupied explicit port fails without fallback', async (t) => {
  const first = server()
  const second = server()
  t.after(async () => {
    await Promise.all([first.stop(), second.stop()])
  })
  const [firstURL, secondURL] = await Promise.all([first.ready, second.ready])
  assert.notEqual(firstURL, secondURL)
  assert.equal(await (await fetch(firstURL)).text(), 'ready')
  assert.equal(await (await fetch(secondURL)).text(), 'ready')
  const occupied = server(Number(new URL(firstURL).port))
  t.after(() => occupied.stop())
  await assert.rejects(occupied.ready, /exited before readiness/)
  assert.equal((await occupied.exited).code, 1)
})

void test('session discovers the API URL before web startup and joins both on cancellation', async () => {
  const controller = new AbortController()
  const urls: string[] = []
  let api: DevChild | undefined
  let web: DevChild | undefined
  const session = runDevSession(
    () => {
      api = server()
      return api
    },
    (apiURL) => {
      assert.equal(apiURL, urls[0])
      web = server()
      return web
    },
    controller.signal,
    (message) => {
      urls.push(message.replace(/^(API|Web): /, ''))
      if (urls.length === 2) controller.abort()
    },
  )
  await assert.rejects(session, /Development stopped/)
  assert.equal(urls.length, 2)
  assert.ok(api)
  assert.ok(web)
  assert.equal((await api.exited).code, 0)
  assert.equal((await web.exited).code, 0)
  await Promise.all(urls.map(assertReleased))
})

void test('web startup failure releases an already running API', async () => {
  let apiURL = ''
  await assert.rejects(
    runDevSession(
      () => server(),
      (url) => {
        apiURL = url
        return startDevChild(
          process.execPath,
          ['-e', 'process.exit(7)'],
          {},
          readyPattern,
          5000,
          quiet,
        )
      },
      new AbortController().signal,
      quiet,
    ),
    /exited before readiness/,
  )
  await assertReleased(apiURL)
})

void test('cancellation during API startup joins the child without starting web', async () => {
  const controller = new AbortController()
  const child = startDevChild(
    process.execPath,
    ['-e', 'setInterval(() => {}, 1000)'],
    {},
    readyPattern,
    5000,
    quiet,
  )
  const session = runDevSession(
    () => child,
    () => {
      throw new Error('web must not start before API readiness')
    },
    controller.signal,
    quiet,
  )
  controller.abort()
  await assert.rejects(session, /Development stopped/)
  assert.equal((await child.exited).signal, 'SIGTERM')
})

void test('API exit during web startup terminates the waiting web child', async () => {
  const api = server()
  let web: DevChild | undefined
  await assert.rejects(
    runDevSession(
      () => api,
      () => {
        web = startDevChild(
          process.execPath,
          ['-e', 'setInterval(() => {}, 1000)'],
          {},
          readyPattern,
          5000,
          quiet,
        )
        void api.stop()
        return web
      },
      new AbortController().signal,
      quiet,
    ),
    /API exited/,
  )
  assert.ok(web)
  assert.equal((await web.exited).signal, 'SIGTERM')
})

void test('startup timeout and forced shutdown leave no listener', async () => {
  const child = server(0, true)
  const url = await child.ready
  await child.stop(50)
  assert.equal((await child.exited).signal, 'SIGKILL')
  await assertReleased(url)
  const silent = startDevChild(
    process.execPath,
    ['-e', 'setInterval(() => {}, 1000)'],
    {},
    readyPattern,
    50,
    quiet,
  )
  try {
    await assert.rejects(silent.ready, /startup timed out/)
  } finally {
    await silent.stop(50)
  }
})

void test('spawn failure and cancellation before startup do not start dependent processes', async () => {
  const missing = startDevChild(
    '/nonexistent-tokeninsights-test-binary',
    [],
    {},
    readyPattern,
    5000,
    quiet,
  )
  await assert.rejects(missing.ready, /ENOENT/)
  await missing.stop()
  const controller = new AbortController()
  controller.abort()
  await assert.rejects(
    runDevSession(
      () => {
        throw new Error('must not start')
      },
      undefined,
      controller.signal,
      quiet,
    ),
    /abort/i,
  )
})

void test('Vite uses the discovered API URL and refuses an occupied explicit web port', async (t) => {
  const api = server()
  t.after(() => api.stop())
  const apiURL = await api.ready
  const webRoot = join(workspaceRoot, 'packages/web')
  const require = createRequire(join(webRoot, 'package.json'))
  const vite = join(dirname(require.resolve('vite/package.json')), 'bin/vite.js')
  function startWeb(port: number) {
    return startDevChild(
      process.execPath,
      [vite, '--port', String(port)],
      {
        cwd: webRoot,
        env: { ...process.env, TOKENINSIGHTS_DEV_API_URL: apiURL, NO_COLOR: '1' },
      },
      /Local:\s+(http:\/\/127\.0\.0\.1:\d+\/)/,
      30_000,
      quiet,
    )
  }
  const web = startWeb(0)
  t.after(() => web.stop())
  const webURL = await web.ready
  assert.notEqual(new URL(webURL).port, '0')
  assert.equal(await (await fetch(new URL('/api/v2/instance', webURL))).text(), 'ready')
  const occupied = startWeb(Number(new URL(webURL).port))
  t.after(() => occupied.stop())
  await assert.rejects(occupied.ready, /exited before readiness/)
  assert.equal((await occupied.exited).code, 1)
  await web.stop()
  await assertReleased(webURL)
})
