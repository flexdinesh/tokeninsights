import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { promisify } from 'node:util'

import { workspacePath } from '../src/workspace.ts'

const execFileAsync = promisify(execFile)

void test('pre-push isolates fixture Git commands from the pushed repository', async (t) => {
  const root = await mkdtemp(join(tmpdir(), 'tokeninsights-pre-push-'))
  t.after(() => rm(root, { recursive: true, force: true }))
  const { stdout } = await execFileAsync('git', ['rev-parse', '--local-env-vars'])
  const env = { ...process.env }
  for (const name of stdout.trim().split('\n')) delete env[name]
  const repository = join(root, 'repository')
  const fixture = join(root, 'fixture')
  const bin = join(root, 'bin')
  await mkdir(bin)
  await execFileAsync('git', ['init', '-q', repository], { env })
  await execFileAsync(
    'git',
    ['-C', repository, 'remote', 'add', 'origin', 'https://example.com/project.git'],
    { env },
  )
  const config = join(repository, '.git', 'config')
  const originalConfig = await readFile(config, 'utf8')
  await writeFile(
    join(bin, 'mise'),
    `#!/bin/sh
set -eu
test "$#" -eq 2
test "$1" = run
test "$2" = check:push
test "$TOKENINSIGHTS_HOOK_SENTINEL" = retained
if read -r ignored; then exit 1; fi
git init -q "$TOKENINSIGHTS_HOOK_FIXTURE"
git -C "$TOKENINSIGHTS_HOOK_FIXTURE" remote add origin https://example.com/fixture.git
`,
    { mode: 0o755 },
  )
  await execFileAsync('sh', [workspacePath('.husky', 'pre-push')], {
    cwd: repository,
    timeout: 10000,
    env: {
      ...env,
      PATH: `${bin}:${env.PATH ?? ''}`,
      GIT_DIR: join(repository, '.git'),
      GIT_WORK_TREE: repository,
      GIT_CONFIG_COUNT: '1',
      GIT_CONFIG_KEY_0: 'core.bare',
      GIT_CONFIG_VALUE_0: 'true',
      TOKENINSIGHTS_HOOK_FIXTURE: fixture,
      TOKENINSIGHTS_HOOK_SENTINEL: 'retained',
    },
  })
  assert.equal(await readFile(config, 'utf8'), originalConfig)
  const remote = await execFileAsync('git', ['-C', fixture, 'remote', 'get-url', 'origin'], { env })
  assert.equal(remote.stdout.trim(), 'https://example.com/fixture.git')
  const bare = await execFileAsync('git', ['-C', fixture, 'rev-parse', '--is-bare-repository'], {
    env,
  })
  assert.equal(bare.stdout.trim(), 'false')
})
