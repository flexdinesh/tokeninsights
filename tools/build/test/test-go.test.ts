import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { mkdtemp, writeFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { promisify } from 'node:util'
import test from 'node:test'
import { workspacePath } from '../src/workspace.ts'

const execute = promisify(execFile)

void test('Go runner bounds package/test concurrency, permits overrides, propagates failure', async (t) => {
  const directory = await mkdtemp(join(tmpdir(), 'tokeninsights-go-runner-'))
  t.after(() => rm(directory, { recursive: true, force: true }))
  await writeFile(
    join(directory, 'go'),
    '#!/bin/sh\nprintf "%s\\n" "$@" "GOMAXPROCS=$GOMAXPROCS"\nexit 23\n',
    { mode: 0o700 },
  )
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    PATH: `${directory}:${process.env.PATH}`,
    TOKENINSIGHTS_TEST_POSTGRES_DSN: 'fixture',
  }
  delete env.TOKENINSIGHTS_CHECK_CONCURRENCY
  delete env.GOMAXPROCS
  for (const override of [false, true]) {
    // Each invocation asserts the real runner's subprocess boundary.
    // eslint-disable-next-line no-await-in-loop
    await assert.rejects(
      execute(
        process.execPath,
        [workspacePath('tools/build/src/test-go.ts'), ...(override ? ['-p=1', '-parallel=3'] : [])],
        {
          env: override ? { ...env, TOKENINSIGHTS_CHECK_CONCURRENCY: '4', GOMAXPROCS: '5' } : env,
        },
      ),
      (error: unknown) => {
        assert.ok(error instanceof Error)
        assert.ok('code' in error && error.code === 23)
        assert.ok('stdout' in error && typeof error.stdout === 'string')
        const lines = error.stdout.trim().split('\n')
        assert.deepEqual(
          lines,
          override
            ? [
                'test',
                '-count=1',
                '-p=4',
                '-parallel=4',
                '-p=1',
                '-parallel=3',
                './...',
                'GOMAXPROCS=5',
              ]
            : ['test', '-count=1', '-p=2', '-parallel=2', './...', 'GOMAXPROCS=2'],
        )
        return true
      },
    )
  }
})
