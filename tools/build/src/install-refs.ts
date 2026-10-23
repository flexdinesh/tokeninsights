import { execFileSync, spawnSync } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { embeddedAssets, workspaceRoot } from './workspace.ts'

const stableTag = /^packages\/cli\/v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$/

function git(root: string, args: string[], env = process.env): string {
  return execFileSync('git', args, { cwd: root, env, encoding: 'utf8' }).trim()
}

function ancestor(root: string, older: string, newer: string): boolean {
  const result = spawnSync('git', ['merge-base', '--is-ancestor', older, newer], {
    cwd: root,
    encoding: 'utf8',
  })
  if (result.status === 0) return true
  if (result.status === 1) return false
  throw new Error(`cannot compare ${older} and ${newer}: ${result.error ?? result.stderr}`)
}

export function releaseSource(root: string, commit: string): string {
  return git(root, ['rev-parse', `${commit}^1`])
}

// An isolated index stages only generated assets over the exact tested source.
// commit-tree leaves the checkout's branch, working tree and real index intact.
export function assetCommit(
  root: string,
  source: string,
  message: string,
  release?: string,
): string {
  if (git(root, ['rev-parse', 'HEAD']) !== source) {
    throw new Error('asset checkout must match the selected source commit')
  }
  const temporary = mkdtempSync(join(tmpdir(), 'tokeninsights-asset-index-'))
  const env = {
    ...process.env,
    GIT_INDEX_FILE: join(temporary, 'index'),
    GIT_AUTHOR_NAME: 'github-actions[bot]',
    GIT_AUTHOR_EMAIL: '41898282+github-actions[bot]@users.noreply.github.com',
    GIT_COMMITTER_NAME: 'github-actions[bot]',
    GIT_COMMITTER_EMAIL: '41898282+github-actions[bot]@users.noreply.github.com',
  }
  try {
    git(root, ['read-tree', source], env)
    git(root, ['add', '-f', '--', embeddedAssets], env)
    const files = git(root, ['ls-files', '--', embeddedAssets], env).split('\n')
    if (
      !files.includes(`${embeddedAssets}/index.html`) ||
      files.some((file) => file.endsWith('/mockServiceWorker.js'))
    ) {
      throw new Error('production dashboard assets missing or contain the mock worker')
    }
    const parents = ['-p', source]
    if (release !== undefined && !ancestor(root, release, source)) parents.push('-p', release)
    const tree = git(root, ['write-tree'], env)
    return git(root, ['commit-tree', tree, ...parents, '-m', message], env)
  } finally {
    rmSync(temporary, { recursive: true, force: true })
  }
}

export function prepareRelease(root: string, source: string, tag: string): string {
  if (!stableTag.test(tag)) throw new Error(`invalid module release tag: ${tag}`)
  const existing = git(root, ['tag', '--list', tag])
  if (existing !== '') {
    const commit = git(root, ['rev-parse', `${tag}^{commit}`])
    if (commit !== source && releaseSource(root, commit) !== source) {
      throw new Error(`${tag} does not belong to source ${source}`)
    }
    return commit
  }
  const commit = assetCommit(root, source, `release: ${tag.replace('packages/cli/', '')}`)
  git(root, ['tag', tag, commit])
  return commit
}

export function publishEdge(root: string, source: string): string | undefined {
  git(root, ['fetch', 'origin', 'main', '--tags'])
  if (!ancestor(root, source, 'FETCH_HEAD')) throw new Error('edge source must belong to main')
  if (git(root, ['rev-parse', 'FETCH_HEAD']) !== source) return undefined
  const remote = git(root, ['ls-remote', '--heads', 'origin', 'refs/heads/edge'])
  let observed = ''
  if (remote !== '') {
    git(root, ['fetch', 'origin', 'refs/heads/edge'])
    observed = git(root, ['rev-parse', 'FETCH_HEAD'])
    const previous = releaseSource(root, observed)
    if (ancestor(root, source, previous)) return undefined
    if (!ancestor(root, previous, source)) throw new Error('edge source history diverged from main')
  }
  const latest = git(root, ['tag', '--list', '--sort=-version:refname'])
    .split('\n')
    .find((tag) => stableTag.test(tag))
  const release = latest === undefined ? undefined : git(root, ['rev-parse', `${latest}^{commit}`])
  const commit = assetCommit(root, source, `edge: ${source}`, release)
  // CI publishes tested source; local pre-push verification must not run here.
  git(root, [
    'push',
    '--no-verify',
    `--force-with-lease=refs/heads/edge:${observed}`,
    'origin',
    `${commit}:refs/heads/edge`,
  ])
  return commit
}

if (process.argv[1] !== undefined && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const source = process.env.GITHUB_SHA
  if (source === undefined || !/^[0-9a-f]{40}$/.test(source))
    throw new Error('GITHUB_SHA must be the selected source commit')
  const mode = process.argv[2]
  if (mode === 'edge') {
    console.log(publishEdge(workspaceRoot, source) ?? 'edge publication skipped')
  } else if (mode === 'release' && process.argv[3] !== undefined) {
    console.log(prepareRelease(workspaceRoot, source, process.argv[3]))
  } else {
    throw new Error('usage: install-refs.ts edge | release packages/cli/vX.Y.Z')
  }
}
