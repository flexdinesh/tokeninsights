import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { chmod, cp, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { pathToFileURL } from 'node:url'
import { setTimeout as pause } from 'node:timers/promises'
import { isCompletion } from '../../../packages/plugin-opencode/src/completion.ts'
import { setupCompletion } from '../../../packages/plugin-opencode/src/lifecycle.ts'
import { registerCompletion } from '../../../packages/plugin-pi/src/completion.ts'
import { registerNativeCompletion } from '../../../packages/plugin-pi/src/lifecycle.ts'
import type { PiCompletionContext } from '../../../packages/plugin-pi/src/lifecycle.ts'
import { CollectorRunner, syncFailure } from '../src/plugin-runner.ts'

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
      assert.deepEqual(actual, {
        args: ['sync', '--wait', '--harness', harness === 'claude' ? 'claude-code' : 'codex'],
        input: '',
      })
      const overridden = await invoke(command, { ...env, TOKENINSIGHTS_BINARY: binary })
      assert.deepEqual(overridden, { stdout: '{}\n', stderr: '' })
      const overrideCapture: unknown = JSON.parse(await readFile(capture, 'utf8'))
      assert.deepEqual(overrideCapture, {
        args: ['sync', '--wait', '--harness', harness === 'claude' ? 'claude-code' : 'codex'],
        input: '',
      })
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

async function withCollector(mode: string, run: (capture: string) => Promise<void>): Promise<void> {
  const temp = await mkdtemp(join(tmpdir(), 'tokeninsights-native-plugin-'))
  const binary = join(temp, "collector with spaces '$ literal")
  const capture = join(temp, 'calls.jsonl')
  const previous = {
    binary: process.env.TOKENINSIGHTS_BINARY,
    capture: process.env.HOOK_CAPTURE,
    mode: process.env.HOOK_MODE,
  }
  const script = `#!${process.execPath}
import { appendFileSync, readFileSync } from 'node:fs';
import { spawn } from 'node:child_process';
appendFileSync(process.env.HOOK_CAPTURE, JSON.stringify({args:process.argv.slice(2),input:readFileSync(0,'utf8'),pid:process.pid})+'\\n');
console.log('synthetic-private-marker'); console.error('synthetic-private-marker');
if(process.env.HOOK_MODE==='fork'){
 spawn(process.execPath,['-e',"process.on('SIGTERM',()=>{});require('node:fs').appendFileSync(process.env.HOOK_CAPTURE,JSON.stringify({descendantPid:process.pid})+'\\\\n');setInterval(()=>{},1000)"],{stdio:'ignore'});
 setInterval(()=>{},1000);
}
else if(process.env.HOOK_MODE==='block'){process.on('SIGTERM',()=>{});setInterval(()=>{},1000)}
else setTimeout(()=>process.exit(process.env.HOOK_MODE==='fail'?2:0),50);
`
  try {
    await writeFile(binary, script)
    await chmod(binary, 0o700)
    process.env.TOKENINSIGHTS_BINARY = binary
    process.env.HOOK_CAPTURE = capture
    process.env.HOOK_MODE = mode
    await run(capture)
  } finally {
    for (const [key, value] of [
      ['TOKENINSIGHTS_BINARY', previous.binary],
      ['HOOK_CAPTURE', previous.capture],
      ['HOOK_MODE', previous.mode],
    ]) {
      if (key === undefined) continue
      if (value === undefined) delete process.env[key]
      else process.env[key] = value
    }
    await rm(temp, { recursive: true, force: true })
  }
}

async function calls(capture: string): Promise<unknown[]> {
  return (await readFile(capture, 'utf8'))
    .trim()
    .split('\n')
    .map((line): unknown => JSON.parse(line))
}

async function waitForCall(capture: string, minimum = 1): Promise<void> {
  const deadline = Date.now() + 3_000
  const probe = async (): Promise<void> => {
    try {
      if ((await calls(capture)).length >= minimum) return
    } catch {
      /* Spawn has not written yet. */
    }
    if (Date.now() >= deadline) assert.fail('collector did not start')
    await pause(10)
    await probe()
  }
  await probe()
}

async function waitForTermination(pid: number): Promise<void> {
  const deadline = Date.now() + 3_000
  const probe = async (): Promise<void> => {
    try {
      if (process.platform === 'linux') {
        // Reparented children may remain as zombies until init reaps them.
        const status = await readFile(`/proc/${pid}/stat`, 'utf8')
        if (status.includes(') Z ')) return
      } else process.kill(pid, 0)
    } catch (error) {
      if (record(error) && (error.code === 'ENOENT' || error.code === 'ESRCH')) return
      throw error
    }
    if (Date.now() >= deadline) assert.fail('descendant remains running')
    await pause(10)
    await probe()
  }
  await probe()
}

void test('finite runner coalesces completions and forwards only sync with empty stdin', async () => {
  await withCollector('ok', async (capture) => {
    const runner = new CollectorRunner({ harness: 'pi' })
    const first = runner.run()
    assert.equal(runner.run(), first)
    await first
    const actual = await calls(capture)
    assert.equal(actual.length, 2)
    assert.ok(record(actual[0]))
    assert.deepEqual(actual[0].args, ['sync', '--wait', '--harness', 'pi'])
    assert.equal(actual[0].input, '')
    assert.ok(record(actual[1]))
    assert.deepEqual(actual[1].args, ['sync', '--wait', '--harness', 'pi'])
    await runner.close()
  })
})

void test('runner deadline escalates termination and leaves no collector child', async () => {
  await withCollector('block', async (capture) => {
    const runner = new CollectorRunner({ harness: 'pi', deadlineMs: 750, killGraceMs: 50 })
    await assert.rejects(runner.run(), new RegExp(syncFailure.replaceAll('.', '\\.')))
    const [actual] = await calls(capture)
    assert.ok(record(actual) && typeof actual.pid === 'number')
    const pid = actual.pid
    assert.throws(() => process.kill(pid, 0))
    await runner.close()
  })
})

void test('missing collector produces only a bounded generic failure', async () => {
  await withCollector('ok', async (capture) => {
    process.env.TOKENINSIGHTS_BINARY = `${capture}-missing-binary`
    const runner = new CollectorRunner({ harness: 'pi' })
    await assert.rejects(runner.run(), { message: syncFailure })
    await runner.close()
    await assert.rejects(readFile(capture))
  })
})

void test(
  'cleanup kills descendants even when the collector exits during graceful termination',
  { skip: process.platform === 'win32' },
  async () => {
    await withCollector('fork', async (capture) => {
      const runner = new CollectorRunner({ harness: 'pi' })
      const pending = runner.run().catch(() => {})
      try {
        await waitForCall(capture, 2)
      } finally {
        await runner.close()
      }
      await pending
      const actual = await calls(capture)
      assert.equal(actual.length, 2)
      assert.ok(record(actual[1]) && typeof actual[1].descendantPid === 'number')
      const pid = actual[1].descendantPid
      try {
        // Signal delivery and the observable process state change asynchronously.
        await waitForTermination(pid)
      } catch (error) {
        try {
          process.kill(pid, 'SIGKILL')
        } catch {
          // Keep the assertion failure if the process exited during cleanup.
        }
        throw error
      }
    })
  },
)

void test('native marketplaces resolve only the corresponding completion packages', async () => {
  const [codex, claude]: unknown[] = await Promise.all([
    readFile(new URL('../../../.agents/plugins/marketplace.json', import.meta.url), 'utf8').then(
      (text): unknown => JSON.parse(text),
    ),
    readFile(new URL('../../../.claude-plugin/marketplace.json', import.meta.url), 'utf8').then(
      (text): unknown => JSON.parse(text),
    ),
  ])
  assert.ok(record(codex) && record(claude))
  assert.equal(codex.name, 'tokeninsights')
  assert.equal(claude.name, 'tokeninsights')
  assert.ok(Array.isArray(codex.plugins) && codex.plugins.length === 1 && record(codex.plugins[0]))
  assert.ok(
    Array.isArray(claude.plugins) && claude.plugins.length === 1 && record(claude.plugins[0]),
  )
  assert.deepEqual(codex.plugins[0].source, { source: 'local', path: './packages/plugin-codex' })
  assert.equal(claude.plugins[0].source, './packages/plugin-claude')
  const manifests: unknown[] = await Promise.all([
    readFile(
      new URL('../../../packages/plugin-codex/.codex-plugin/plugin.json', import.meta.url),
      'utf8',
    ).then((text): unknown => JSON.parse(text)),
    readFile(
      new URL('../../../packages/plugin-claude/.claude-plugin/plugin.json', import.meta.url),
      'utf8',
    ).then((text): unknown => JSON.parse(text)),
  ])
  for (const manifest of manifests) {
    assert.ok(record(manifest))
    assert.equal(manifest.name, 'tokeninsights')
  }
})

void test('Pi native registration stays inert until settled and isolates generic failures', async () => {
  await withCollector('fail', async (capture) => {
    const handlers = new Map<
      string,
      (event: unknown, context: PiCompletionContext) => Promise<void>
    >()
    const errors: string[] = []
    registerNativeCompletion({
      on(event, handler) {
        handlers.set(event, handler)
      },
    })
    assert.deepEqual([...handlers.keys()], ['agent_settled', 'session_shutdown'])
    await assert.rejects(readFile(capture))
    const settled = handlers.get('agent_settled')
    const shutdown = handlers.get('session_shutdown')
    assert.ok(settled !== undefined && shutdown !== undefined)
    const context: PiCompletionContext = {
      hasUI: true,
      ui: {
        notify(message) {
          errors.push(message)
        },
      },
    }
    await settled({ assistant: 'synthetic-private-marker' }, context)
    await waitForCall(capture)
    const deadline = Date.now() + 5000
    const waitForFailure = async (): Promise<void> => {
      if (errors.length > 0) return
      if (Date.now() >= deadline) assert.fail('sync failure not reported')
      await pause(10)
      await waitForFailure()
    }
    await waitForFailure()
    assert.deepEqual(errors, [syncFailure])
    const actual = await calls(capture)
    assert.equal(actual.length, 1)
    assert.ok(record(actual[0]))
    assert.deepEqual(actual[0].args, ['sync', '--wait', '--harness', 'pi'])
    assert.equal(actual[0].input, '')
    await shutdown({}, context)
  })
})

void test('OpenCode native lifecycle coalesces idle triggers and kills active work on cleanup', async () => {
  await withCollector('block', async (capture) => {
    let aborted = false
    const cleanup = setupCompletion({
      event: {
        async *subscribe({ signal }) {
          yield { type: 'session.idle', data: { sessionID: 'synthetic' } }
          yield {
            type: 'session.status',
            data: { sessionID: 'synthetic', status: { type: 'busy' } },
          }
          for (let i = 0; i < 10; i++)
            yield {
              type: 'session.status',
              data: { sessionID: 'synthetic', status: { type: 'idle' } },
              transcript: 'synthetic-private-marker',
            }
          await new Promise<void>((resolve) =>
            signal.addEventListener(
              'abort',
              () => {
                aborted = true
                resolve()
              },
              { once: true },
            ),
          )
        },
      },
    })
    await waitForCall(capture)
    await cleanup()
    assert.equal(aborted, true)
    const actual = await calls(capture)
    assert.equal(actual.length, 1)
    assert.ok(record(actual[0]) && typeof actual[0].pid === 'number')
    assert.deepEqual(actual[0].args, ['sync', '--wait', '--harness', 'opencode'])
    assert.equal(actual[0].input, '')
    const pid = actual[0].pid
    assert.throws(() => process.kill(pid, 0))
  })
})

void test('ready-built native artifacts load outside workspace without SDK or build tools', async () => {
  const temp = await mkdtemp(join(tmpdir(), 'tokeninsights-isolated-plugin-'))
  try {
    await Promise.all(
      ['pi', 'opencode'].map(async (harness) => {
        const destination = join(temp, harness)
        await mkdir(destination)
        const source = new URL(`../../../packages/plugin-${harness}/`, import.meta.url)
        await Promise.all([
          cp(new URL('dist/', source), join(destination, 'dist'), { recursive: true }),
          cp(new URL('package.json', source), join(destination, 'package.json')),
        ])
        const imported: unknown = await import(
          pathToFileURL(join(destination, 'dist', 'index.js')).href
        )
        assert.ok(record(imported))
        if (harness === 'pi') assert.equal(typeof imported.default, 'function')
        else
          assert.ok(
            record(imported.default) &&
              imported.default.id === 'tokeninsights' &&
              typeof imported.default.setup === 'function',
          )
        const runner = await readFile(join(destination, 'dist', 'runner.js'), 'utf8')
        assert.equal(runner.includes('tools/build'), false)
      }),
    )
  } finally {
    await rm(temp, { recursive: true, force: true })
  }
})
