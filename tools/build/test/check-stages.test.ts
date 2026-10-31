import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test, { type TestContext } from 'node:test'
import { promisify } from 'node:util'

import { workspacePath } from '../src/workspace.ts'

const execute = promisify(execFile)

// The fake mise records each invocation and behaves according to the task name.
const fakeMise = `#!/bin/sh
printf '%s\\n' "$*" >> "$TOKENINSIGHTS_FAKE_CALLS"
task="$5"
case "$task" in
  pass-*) echo "stdout of $task"; echo "stderr of $task" >&2 ;;
  fail) i=1; while [ "$i" -le 150 ]; do echo "line $i"; i=$((i + 1)); done; exit 7 ;;
  slow) exec sleep 30 ;;
  *) exit 99 ;;
esac
`

interface Run {
  code: number
  stdout: string
  calls: string[]
  directory: string
}

async function runStages(t: TestContext, stages: string[], verbose = false): Promise<Run> {
  const directory = await mkdtemp(join(tmpdir(), 'tokeninsights-check-stages-'))
  t.after(() => rm(directory, { recursive: true, force: true }))
  const bin = join(directory, 'bin')
  await mkdir(bin)
  await writeFile(join(bin, 'mise'), fakeMise, { mode: 0o755 })
  await mkdir(join(directory, '.check-logs'))
  await writeFile(join(directory, '.check-logs', 'stale.log'), 'previous run\n')
  const calls = join(directory, 'calls.txt')
  await writeFile(calls, '')
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    PATH: `${bin}:${process.env.PATH ?? ''}`,
    TOKENINSIGHTS_FAKE_CALLS: calls,
    TOKENINSIGHTS_CHECK_CONCURRENCY: '2',
  }
  delete env.TOKENINSIGHTS_CHECK_VERBOSE
  if (verbose) env.TOKENINSIGHTS_CHECK_VERBOSE = '1'
  const result = await execute(
    process.execPath,
    [workspacePath('tools/build/src/check-stages.ts'), ...stages],
    { cwd: directory, env, timeout: 20000 },
  ).then(
    ({ stdout }) => ({ code: 0, stdout }),
    (error: unknown) => {
      assert.ok(error instanceof Error)
      assert.ok('code' in error && typeof error.code === 'number')
      assert.ok('stdout' in error && typeof error.stdout === 'string')
      return { code: error.code, stdout: error.stdout }
    },
  )
  const recorded = (await readFile(calls, 'utf8')).trim()
  return { ...result, calls: recorded === '' ? [] : recorded.split('\n'), directory }
}

async function log(run: Run, name: string): Promise<string> {
  return readFile(join(run.directory, '.check-logs', name), 'utf8')
}

void test('quiet success prints one line per phase and logs complete output', async (t) => {
  const run = await runStages(t, ['pass-a,pass-b', 'pass-c'])
  assert.equal(run.code, 0)
  const lines = run.stdout.trim().split('\n')
  assert.equal(lines.length, 3)
  for (const task of ['pass-a', 'pass-b', 'pass-c']) {
    assert.ok(
      lines.some((line) => new RegExp(`^ok ${task} \\d+s$`).test(line)),
      run.stdout,
    )
  }
  assert.ok(lines[2]?.startsWith('ok pass-c '))
  assert.deepEqual(
    run.calls.toSorted(),
    ['pass-a', 'pass-b', 'pass-c'].map((task) => `run --no-deps --jobs 1 ${task}`),
  )
  assert.deepEqual(await readdir(join(run.directory, '.check-logs')), [
    '01-pass-a.log',
    '02-pass-b.log',
    '03-pass-c.log',
  ])
  const output = await log(run, '01-pass-a.log')
  assert.match(output, /^stdout of pass-a$/m)
  assert.match(output, /^stderr of pass-a$/m)
})

void test('failure prints tail, log path and rerun command, keeps exit code, stops pipeline', async (t) => {
  const run = await runStages(t, ['pass-a', 'fail', 'pass-c'])
  assert.equal(run.code, 7)
  assert.deepEqual(run.calls, ['run --no-deps --jobs 1 pass-a', 'run --no-deps --jobs 1 fail'])
  const lines = run.stdout.trim().split('\n')
  assert.match(lines[0] ?? '', /^ok pass-a \d+s$/)
  assert.match(lines[1] ?? '', /^FAIL fail \d+s \(exit 7\)$/)
  assert.equal(lines[2], '--- last 100 lines of .check-logs/02-fail.log ---')
  assert.deepEqual(
    lines.slice(3, 103),
    Array.from({ length: 100 }, (_, index) => `line ${index + 51}`),
  )
  assert.deepEqual(lines.slice(103), [
    '---',
    'log: .check-logs/02-fail.log',
    'rerun: mise run fail',
  ])
  const output = (await log(run, '02-fail.log')).trim().split('\n')
  assert.equal(output.length, 150)
  assert.equal(output[0], 'line 1')
})

void test('a parallel failure stops running siblings', async (t) => {
  const run = await runStages(t, ['slow,fail', 'pass-c'])
  assert.equal(run.code, 7)
  assert.ok(!run.calls.includes('run --no-deps --jobs 1 pass-c'))
  assert.match(run.stdout, /^FAIL fail \d+s \(exit 7\)$/m)
  assert.match(run.stdout, /^stopped slow$/m)
  assert.match(run.stdout, /^rerun: mise run fail$/m)
})

void test('verbose mode streams prefixed output and still logs', async (t) => {
  const run = await runStages(t, ['pass-a'], true)
  assert.equal(run.code, 0)
  assert.match(run.stdout, /^\[pass-a\] stdout of pass-a$/m)
  assert.match(run.stdout, /^\[pass-a\] stderr of pass-a$/m)
  assert.match(run.stdout, /^ok pass-a \d+s$/m)
  assert.match(await log(run, '01-pass-a.log'), /^stdout of pass-a$/m)
})
