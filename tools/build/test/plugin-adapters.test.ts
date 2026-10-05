import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { chmod, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { isCompletion } from '../../../packages/plugin-opencode/src/completion.ts'
import { registerCompletion } from '../../../packages/plugin-pi/src/completion.ts'

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

async function commandHook(harness: string): Promise<string> {
  const config: unknown = JSON.parse(
    await readFile(
      new URL(`../../../packages/plugin-${harness}/hooks/hooks.json`, import.meta.url),
      'utf8',
    ),
  )
  assert.ok(record(config) && record(config.hooks))
  assert.deepEqual(Object.keys(config.hooks), ['Stop'])
  const stops: unknown = config.hooks.Stop
  assert.ok(Array.isArray(stops) && stops.length === 1 && record(stops[0]))
  const hooks: unknown = stops[0].hooks
  assert.ok(Array.isArray(hooks) && hooks.length === 1 && record(hooks[0]))
  assert.equal(hooks[0].type, 'command')
  assert.equal(hooks[0].timeout, 60)
  assert.equal(hooks[0].async, undefined)
  assert.ok(typeof hooks[0].command === 'string')
  return hooks[0].command
}

function invoke(
  command: string,
  env: NodeJS.ProcessEnv,
): Promise<{ stdout: string; stderr: string }> {
  return new Promise((resolve, reject) => {
    const child = execFile(
      'sh',
      ['-c', command],
      { env, timeout: 5_000 },
      (error, stdout, stderr) => {
        if (error !== null) reject(error)
        else resolve({ stdout, stderr })
      },
    )
    child.stdin?.on('error', () => {
      // The wrapper deliberately closes hook input without consuming it.
    })
    child.stdin?.end('{"last_assistant_message":"synthetic-private-marker"}')
  })
}

for (const harness of ['codex', 'claude']) {
  void test(`${harness} Stop runs only sync, discards hook input and CLI output`, async () => {
    const command = await commandHook(harness)
    const temp = await mkdtemp(join(tmpdir(), 'tokeninsights-hook-'))
    const capture = join(temp, 'capture.json')
    const binary = join(temp, "binary with spaces '$ literal")
    const script = `#!${process.execPath}
import { readFileSync, writeFileSync } from 'node:fs';
writeFileSync(process.env.HOOK_CAPTURE, JSON.stringify({ args: process.argv.slice(2), input: readFileSync(0, 'utf8') }));
console.log('synthetic-private-marker');
console.error('synthetic-private-marker');
process.exit(Number(process.env.HOOK_EXIT || 0));
`
    try {
      await Promise.all([writeFile(binary, script), writeFile(join(temp, 'tokeninsights'), script)])
      await Promise.all([chmod(binary, 0o700), chmod(join(temp, 'tokeninsights'), 0o700)])
      const env: NodeJS.ProcessEnv = {
        ...process.env,
        PATH: `${temp}:${process.env.PATH ?? ''}`,
        HOOK_CAPTURE: capture,
      }
      delete env.TOKENINSIGHTS_BINARY
      const result = await invoke(command, env)
      assert.deepEqual(result, { stdout: '{}\n', stderr: '' })
      const actual: unknown = JSON.parse(await readFile(capture, 'utf8'))
      assert.deepEqual(actual, { args: ['sync'], input: '' })
      const overridden = await invoke(command, { ...env, TOKENINSIGHTS_BINARY: binary })
      assert.deepEqual(overridden, { stdout: '{}\n', stderr: '' })
      const overrideCapture: unknown = JSON.parse(await readFile(capture, 'utf8'))
      assert.deepEqual(overrideCapture, { args: ['sync'], input: '' })
      const failed = await invoke(command, {
        ...env,
        TOKENINSIGHTS_BINARY: binary,
        HOOK_EXIT: '2',
      })
      assert.deepEqual(failed, {
        stdout: '{}\n',
        stderr: 'TokenInsights sync failed; run tokeninsights sync for details.\n',
      })
      const missing = await invoke(command, {
        ...env,
        TOKENINSIGHTS_BINARY: '/missing/tokeninsights',
      })
      assert.deepEqual(missing, failed)
    } finally {
      await rm(temp, { recursive: true, force: true })
    }
  })
}

void test('Pi routing waits for settled sync and consumes no event content', async () => {
  let handler: ((event: unknown) => Promise<void>) | undefined
  let started = false
  let completed = false
  let finish: (() => void) | undefined
  const completion = new Promise<void>((resolve) => {
    finish = resolve
  })
  registerCompletion(
    {
      on(event, callback) {
        assert.equal(event, 'agent_settled')
        handler = callback
      },
    },
    async () => {
      started = true
      await completion
      completed = true
    },
  )
  assert.ok(handler !== undefined && finish !== undefined)
  const pending = handler({ assistant: 'synthetic-private-marker' })
  assert.equal(started, true)
  assert.equal(completed, false)
  finish()
  await pending
  assert.equal(completed, true)
})

void test('OpenCode V2 routing accepts idle statuses without inspecting transcripts', () => {
  const completion = {
    type: 'session.status',
    data: { sessionID: 'synthetic-session', status: { type: 'idle' } },
  }
  assert.equal(isCompletion(completion), true)
  assert.equal(isCompletion({ ...completion, transcript: 'synthetic-private-marker' }), true)
  for (const value of [
    null,
    [],
    {},
    { type: 'session.idle', data: { sessionID: 'synthetic-session' } },
    { type: 'session.status', data: { sessionID: '', status: { type: 'idle' } } },
    { type: 'session.status', data: { sessionID: 'synthetic-session', status: { type: 'busy' } } },
    { type: 'session.status', data: { sessionID: 'synthetic-session', status: { type: 'retry' } } },
  ]) {
    assert.equal(isCompletion(value), false)
  }
})
