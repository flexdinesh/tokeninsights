import { spawn } from 'node:child_process'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { hostname, tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { expect, test } from '@playwright/test'
import { InstanceResponseV2 } from '../src/generated/api'

function startCLI(
  args: string[],
  env: NodeJS.ProcessEnv,
): Promise<{ origin: string; stop: () => Promise<void> }> {
  return new Promise((resolveRun, reject) => {
    const child = spawn(resolve('../cli/bin/tokeninsights'), args, {
      env,
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    let output = ''
    let ready = false
    const deadline = setTimeout(() => {
      child.kill('SIGTERM')
      reject(new Error('local web startup timed out'))
    }, 10000)
    child.stdout.on('data', (chunk: Buffer) => {
      output += chunk.toString()
      const origin = /Dashboard: (http:\/\/[^\s]+)/u.exec(output)?.[1]
      if (origin && !ready) {
        ready = true
        clearTimeout(deadline)
        resolveRun({
          origin,
          stop: () =>
            new Promise<void>((resolveStop) => {
              if (child.exitCode !== null || child.signalCode !== null) {
                resolveStop()
                return
              }
              child.once('exit', () => resolveStop())
              child.kill('SIGTERM')
            }),
        })
      }
    })
    child.stderr.resume()
    child.once('error', (error) => {
      clearTimeout(deadline)
      reject(error)
    })
    child.once('exit', (code) => {
      clearTimeout(deadline)
      if (!ready) reject(new Error(`Local web exited before readiness (${code})`))
    })
  })
}

async function localEnvironment(home: string) {
  const runtime = join(home, 'run')
  await mkdir(runtime, { mode: 0o700 })
  return {
    ...process.env,
    HOME: home,
    TOKENINSIGHTS_MODE: 'single-process',
    TOKENINSIGHTS_SERVER_URL: '',
    TOKENINSIGHTS_ACCESS_TOKEN: '',
    XDG_RUNTIME_DIR: runtime,
    XDG_CONFIG_HOME: join(home, 'config'),
    XDG_DATA_HOME: join(home, 'data'),
    XDG_STATE_HOME: join(home, 'state'),
    CODEX_HOME: join(home, 'codex'),
    CLAUDE_CONFIG_DIR: join(home, 'claude'),
    DISPLAY: '',
    WAYLAND_DISPLAY: '',
  }
}

test('foreground web collects directly, keeps Reload read-only and stops with its command', async ({
  page,
}) => {
  const home = await mkdtemp(join(tmpdir(), 'ti-managed-web-'))
  const env = await localEnvironment(home)
  const sessions = join(home, '.pi/agent/sessions')
  await mkdir(sessions, { recursive: true })
  await writeFile(
    join(sessions, 'synthetic.jsonl'),
    [
      { type: 'session', id: 'managed-session' },
      {
        type: 'message',
        id: 'managed-message',
        message: {
          role: 'assistant',
          timestamp: Date.now(),
          provider: 'synthetic-provider',
          model: 'synthetic-model',
          usage: { input: 100, output: 20 },
        },
      },
    ]
      .map((record) => JSON.stringify(record))
      .join('\n'),
  )
  const running = await startCLI(['web', '--open=false', '--host', '127.0.0.1', '--port', '0'], env)
  const origin = running.origin
  try {
    const descriptor = InstanceResponseV2.parse(
      await (await page.request.get(origin + '/api/v2/instance')).json(),
    )
    expect(descriptor.serverKind).toBe('personal')
    expect(descriptor.capabilities).toContain('collector-progress')
    expect(descriptor.capabilities).toContain('dashboard-reload')
    expect(descriptor.hostname).toBe(hostname())
    expect(
      (await page.request.post(origin + '/api/v3/ingestion/batches', { data: {} })).status(),
    ).toBe(404)
    const methods: string[] = []
    page.on('request', (request) => {
      if (new URL(request.url()).pathname.startsWith('/api/')) methods.push(request.method())
    })
    await page.goto(origin + '/tokens')
    await expect(page.getByLabel('Total tokens: 120', { exact: true })).toBeVisible()
    await expect(page.getByTitle(/Local machine running this dashboard/)).toHaveText(hostname())
    // A later source must remain uncollected when Reload refreshes queries.
    await writeFile(
      join(sessions, 'later.jsonl'),
      [
        { type: 'session', id: 'later-session' },
        {
          type: 'message',
          id: 'later-message',
          message: {
            role: 'assistant',
            timestamp: Date.now(),
            provider: 'synthetic-provider',
            model: 'synthetic-model',
            usage: { input: 500, output: 100 },
          },
        },
      ]
        .map((record) => JSON.stringify(record))
        .join('\n'),
    )
    await page.getByRole('button', { name: 'Reload', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Reload', exact: true })).toBeEnabled()
    await expect(page.getByLabel('Total tokens: 120', { exact: true })).toBeVisible()
    expect(methods.every((method) => method === 'GET')).toBe(true)
  } finally {
    await running.stop()
    await rm(home, { recursive: true, force: true })
  }
  await expect
    .poll(async () => {
      try {
        await page.request.get(origin + '/api/v2/instance')
        return false
      } catch {
        return true
      }
    })
    .toBe(true)
})

test('local web with sync disabled still advertises query Reload and never captures source usage', async ({
  page,
}) => {
  const home = await mkdtemp(join(tmpdir(), 'ti-saved-web-'))
  const env = await localEnvironment(home)
  const sessions = join(home, '.pi/agent/sessions')
  await mkdir(sessions, { recursive: true })
  await writeFile(
    join(sessions, 'uncollected.jsonl'),
    [
      { type: 'session', id: 'uncollected-session' },
      {
        type: 'message',
        id: 'uncollected-message',
        message: {
          role: 'assistant',
          timestamp: Date.now(),
          provider: 'synthetic-provider',
          model: 'synthetic-model',
          usage: { input: 100, output: 20 },
        },
      },
    ]
      .map((record) => JSON.stringify(record))
      .join('\n'),
  )
  const running = await startCLI(
    ['web', '--sync=false', '--open=false', '--host', '127.0.0.1', '--port', '0'],
    env,
  )
  try {
    const descriptor = InstanceResponseV2.parse(
      await (await page.request.get(running.origin + '/api/v2/instance')).json(),
    )
    expect(descriptor.capabilities).toContain('dashboard-reload')
    expect(descriptor.hostname).toBe(hostname())
    await page.goto(running.origin + '/tokens')
    await expect(page.getByText('No usage saved yet. Run tokeninsights sync.')).toBeVisible()
    await expect(page.getByRole('region', { name: 'Collector progress' })).toHaveCount(0)
    await page.getByRole('button', { name: 'Reload', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Reload', exact: true })).toBeEnabled()
    await expect(page.getByText('No usage saved yet. Run tokeninsights sync.')).toBeVisible()
    await expect(page.getByLabel('Total tokens: 120', { exact: true })).toHaveCount(0)
  } finally {
    await running.stop()
    await rm(home, { recursive: true, force: true })
  }
})
