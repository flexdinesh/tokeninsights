import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test, { type TestContext } from 'node:test'
import { fileURLToPath } from 'node:url'

const script = fileURLToPath(new URL('../src/publish-dev.ts', import.meta.url))
const gitEnv = { ...process.env, GIT_CONFIG_GLOBAL: '/dev/null', GIT_CONFIG_NOSYSTEM: '1' }

function gitAt(cwd: string, ...args: string[]): string {
  return execFileSync('git', args, {
    cwd,
    env: gitEnv,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  }).trim()
}

async function repository(t: TestContext) {
  const root = await mkdtemp(join(tmpdir(), 'tokeninsights-publish-dev-'))
  t.after(() => rm(root, { recursive: true, force: true }))
  const remote = join(root, 'remote.git')
  const checkout = join(root, 'checkout')
  const git = (...args: string[]) => gitAt(checkout, ...args)
  gitAt(root, 'init', '--bare', '--initial-branch=main', remote)
  gitAt(root, 'clone', remote, checkout)
  git('config', 'user.name', 'Release test')
  git('config', 'user.email', 'release@example.invalid')
  git('config', 'commit.gpgsign', 'false')
  git('commit', '--allow-empty', '-m', 'Stable')
  const stable = git('rev-parse', 'HEAD')
  git('tag', 'packages/cli/v0.1.0')
  git('commit', '--allow-empty', '-m', 'Main change')
  const commit = git('rev-parse', 'HEAD')
  git('push', 'origin', 'main', 'refs/tags/packages/cli/v0.1.0')

  return {
    git,
    stable,
    commit,
    remoteGit: (...args: string[]) => gitAt(remote, ...args),
    publish: (sha = commit) =>
      execFileSync(process.execPath, [script], {
        cwd: checkout,
        env: { ...gitEnv, GITHUB_SHA: sha },
        encoding: 'utf8',
        stdio: ['ignore', 'pipe', 'pipe'],
      }),
  }
}

void test('publishes main as dev without creating or changing stable tags', async (t) => {
  const repo = await repository(t)
  repo.publish()

  assert.equal(repo.remoteGit('rev-parse', 'refs/heads/dev'), repo.commit)
  assert.equal(repo.remoteGit('rev-parse', 'refs/heads/main'), repo.commit)
  assert.equal(repo.remoteGit('tag', '--list'), 'packages/cli/v0.1.0')
  assert.equal(repo.remoteGit('rev-parse', 'refs/tags/packages/cli/v0.1.0'), repo.stable)
})

void test('replaces obsolete divergent dev and supports reruns', async (t) => {
  const repo = await repository(t)
  repo.git('checkout', '-b', 'dev', repo.stable)
  repo.git('commit', '--allow-empty', '-m', 'Obsolete dev')
  repo.git('push', 'origin', 'dev')
  repo.git('checkout', 'main')

  repo.publish()
  repo.publish()

  assert.equal(repo.remoteGit('rev-parse', 'refs/heads/dev'), repo.commit)
})

void test('publishes successive main pushes and skips older CI reruns', async (t) => {
  const repo = await repository(t)
  repo.publish()
  repo.git('commit', '--allow-empty', '-m', 'Newer main')
  const newer = repo.git('rev-parse', 'HEAD')
  repo.git('push', 'origin', 'main')
  repo.publish(newer)
  assert.equal(repo.remoteGit('rev-parse', 'refs/heads/dev'), newer)

  repo.git('checkout', '--detach', repo.commit)
  assert.match(repo.publish(), /Skipping superseded main commit/)
  assert.equal(repo.remoteGit('rev-parse', 'refs/heads/dev'), newer)
})

void test('superseded main does not publish while newer CI is pending', async (t) => {
  const repo = await repository(t)
  repo.git('commit', '--allow-empty', '-m', 'Newer main')
  repo.git('push', 'origin', 'main')
  repo.git('checkout', '--detach', repo.commit)

  assert.match(repo.publish(), /Skipping superseded main commit/)
  assert.equal(repo.git('ls-remote', '--heads', 'origin', 'refs/heads/dev'), '')
})

void test('rejects missing or mismatched verified commit without publishing', async (t) => {
  const repo = await repository(t)
  assert.throws(() => repo.publish(''), /Checkout must match the verified GITHUB_SHA/)
  assert.throws(() => repo.publish(repo.stable), /Checkout must match the verified GITHUB_SHA/)
  assert.equal(repo.git('ls-remote', '--heads', 'origin', 'refs/heads/dev'), '')
})

void test('reports rejected branch updates without changing dev', async (t) => {
  const repo = await repository(t)
  repo.git('checkout', '-b', 'dev', repo.stable)
  repo.git('commit', '--allow-empty', '-m', 'Obsolete dev')
  const obsolete = repo.git('rev-parse', 'HEAD')
  repo.git('push', 'origin', 'dev')
  repo.git('checkout', 'main')
  repo.remoteGit('config', 'receive.denyNonFastForwards', 'true')

  assert.throws(() => repo.publish(), /non-fast-forward/)
  assert.equal(repo.remoteGit('rev-parse', 'refs/heads/dev'), obsolete)
})

void test('rechecks refs and retries when dev changes during publication', async (t) => {
  const repo = await repository(t)
  const concurrent = repo.git(
    'commit-tree',
    'HEAD^{tree}',
    '-p',
    repo.stable,
    '-m',
    'Concurrent dev update',
  )
  repo.git('push', 'origin', `${repo.stable}:refs/heads/dev`, `${concurrent}:refs/heads/other`)
  const hook = join(repo.git('rev-parse', '--absolute-git-dir'), 'hooks', 'pre-push')
  const remote = repo.git('remote', 'get-url', 'origin')
  // Move the remote ref after its advertisement, forcing a real Git lease rejection.
  await writeFile(
    hook,
    `#!/usr/bin/env node
const { execFileSync } = require('node:child_process')
const { unlinkSync } = require('node:fs')
unlinkSync(__filename)
execFileSync('git', ['--git-dir', ${JSON.stringify(remote)}, 'update-ref', 'refs/heads/dev', ${JSON.stringify(concurrent)}])
`,
    { mode: 0o755 },
  )

  assert.match(repo.publish(), /rechecking main and dev before retry/)
  assert.equal(repo.remoteGit('rev-parse', 'refs/heads/dev'), repo.commit)
})
