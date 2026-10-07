import { spawn } from 'node:child_process'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { expect, test } from '@playwright/test'
import { CollectorProgressResponse, InstanceResponseV2 } from '../src/generated/api'

function runCLI(args: string[], env: NodeJS.ProcessEnv): Promise<string> {
  return new Promise((resolveRun, reject) => {
    const child = spawn(resolve('../cli/bin/tokeninsights'), args, {
      env,
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    let output = ''
    let diagnostics = ''
    child.stdout.on('data', (chunk: Buffer) => {
      output += chunk.toString()
    })
    child.stderr.on('data', (chunk: Buffer) => {
      diagnostics += chunk.toString()
    })
    child.once('error', reject)
    child.once('exit', (code) => {
      if (code === 0) resolveRun(output)
      else reject(new Error(`Managed CLI fixture failed (${code}): ${diagnostics}`))
    })
  })
}

test('one web command creates personal server, syncs, and exposes local progress while Reload stays read-only', async ({
  page,
}) => {
  const home = await mkdtemp(join(tmpdir(), 'ti-managed-web-'))
  const runtime = join(home, 'run')
  await mkdir(runtime, { mode: 0o700 })
  const env = {
    ...process.env,
    HOME: home,
    XDG_RUNTIME_DIR: runtime,
    XDG_CONFIG_HOME: join(home, 'config'),
    XDG_DATA_HOME: join(home, 'data'),
    XDG_STATE_HOME: join(home, 'state'),
    CODEX_HOME: join(home, 'codex'),
    CLAUDE_CONFIG_DIR: join(home, 'claude'),
    DISPLAY: '',
    WAYLAND_DISPLAY: '',
  }
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
  try {
    const output = await runCLI(['web', '--host', '127.0.0.1', '--port', '0'], env)
    const origin = /Dashboard: (http:\/\/[^\s]+)/u.exec(output)?.[1]
    if (!origin) throw new Error('Managed web command omitted dashboard URL')
    const descriptor = InstanceResponseV2.parse(
      await (await page.request.get(origin + '/api/v2/instance')).json(),
    )
    expect(descriptor.serverKind).toBe('personal')
    expect(descriptor.capabilities).toContain('collector-progress')
    const progress = CollectorProgressResponse.parse(
      await (await page.request.get(origin + '/api/v2/collector-progress')).json(),
    )
    expect(progress.attempts.some((attempt) => attempt.stage === 'accepted')).toBe(true)
    const methods: string[] = []
    page.on('request', (request) => {
      if (new URL(request.url()).pathname.startsWith('/api/')) methods.push(request.method())
    })
    await page.goto(origin + '/tokens')
    await expect(page.getByLabel('Total tokens: 120', { exact: true })).toBeVisible()
    await expect(page.getByRole('region', { name: 'Collector progress' })).toContainText(
      'Usage accepted',
    )
    await page.getByRole('button', { name: 'Reload', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Reload', exact: true })).toBeEnabled()
    await expect(page.getByLabel('Total tokens: 120', { exact: true })).toBeVisible()
    expect(methods.every((method) => method === 'GET')).toBe(true)
  } finally {
    try {
      await runCLI(['service', 'stop'], env)
    } finally {
      await rm(home, { recursive: true, force: true })
    }
  }
})
