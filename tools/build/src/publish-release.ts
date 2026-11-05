import { execFileSync, spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { readFile, readdir } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import { archiveName, products } from './build-products.ts'
import { parseTarget } from './build-desktop.ts'
import { workspaceRoot } from './workspace.ts'

const stableVersion = /^(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$/
export function releaseTag(version: string): string {
  if (!stableVersion.test(version)) throw new Error('version must be x.y.z without leading zeros')
  return `v${version}`
}
export type ReleaseChannel = 'stable' | 'main'
export function releasePlatforms(channel: ReleaseChannel): string[] {
  return channel === 'main'
    ? ['linux/amd64', 'linux/arm64', 'darwin/arm64']
    : ['linux/amd64', 'linux/arm64', 'darwin/amd64', 'darwin/arm64']
}
export function expectedArchives(channel: ReleaseChannel): string[] {
  return releasePlatforms(channel)
    .flatMap((platform) => products.map((product) => archiveName(product, parseTarget(platform))))
    .toSorted()
}
export function verifyArchiveMetadata(
  path: string,
  name: string,
  expected: { version: string; commit: string },
): void {
  const root = name.slice(0, -'.tar.gz'.length)
  const entries = execFileSync('tar', ['-tzf', path], { encoding: 'utf8' }).trim().split('\n')
  if (
    entries.some(
      (entry) =>
        entry !== `${root}/` &&
        (!entry.startsWith(`${root}/`) ||
          entry.split('/').some((part) => part === '.' || part === '..')),
    )
  )
    throw new Error(`unsafe product archive ${name}`)
  const value: unknown = JSON.parse(
    execFileSync('tar', ['-xOf', path, `${root}/build-info.json`], { encoding: 'utf8' }),
  )
  const [slug, os, arch] = root.split('_')
  if (
    typeof value !== 'object' ||
    value === null ||
    !('format' in value) ||
    value.format !== 1 ||
    !('product' in value) ||
    value.product !== slug?.replace('tokeninsights-', '') ||
    !('os' in value) ||
    value.os !== os ||
    !('arch' in value) ||
    value.arch !== arch ||
    !('version' in value) ||
    value.version !== expected.version ||
    !('commit' in value) ||
    value.commit !== expected.commit
  )
    throw new Error(`incompatible source metadata in ${name}`)
}
export async function verifyReleaseAssets(
  out: string,
  channel: ReleaseChannel,
  metadata?: { version: string; commit: string },
): Promise<void> {
  const archives = expectedArchives(channel)
  const names = (await readdir(out)).toSorted()
  const expected = [...archives, 'checksums.txt'].toSorted()
  if (JSON.stringify(names) !== JSON.stringify(expected))
    throw new Error(
      `${channel} release requires exactly ${archives.length} product archives and checksums.txt`,
    )
  const checksums = (await readFile(join(out, 'checksums.txt'), 'utf8')).trim().split('\n')
  if (checksums.length !== archives.length) throw new Error('unexpected checksum count')
  await Promise.all(
    archives.map(async (name) => {
      const entries = checksums
        .map((line) => /^([0-9a-f]{64})  (\S+)$/.exec(line))
        .filter((entry) => entry?.[2] === name)
      if (entries.length !== 1) throw new Error(`requires unique checksum for ${name}`)
      const hash = createHash('sha256')
        .update(await readFile(join(out, name)))
        .digest('hex')
      if (entries[0]?.[1] !== hash) throw new Error(`checksum mismatch for ${name}`)
      if (metadata !== undefined) verifyArchiveMetadata(join(out, name), name, metadata)
    }),
  )
}
export function prepareTag(version: string, cwd = workspaceRoot): string {
  const tag = releaseTag(version)
  const git = (...args: string[]) => execFileSync('git', args, { cwd, encoding: 'utf8' }).trim()
  const source = git('rev-parse', 'HEAD')
  const remote = git('ls-remote', '--tags', 'origin', `refs/tags/${tag}`)
  if (remote !== '') {
    const existing = remote.split(/\s+/)[0]
    if (existing !== source) throw new Error(`${tag} already belongs to another source commit`)
    return tag
  }
  // Lightweight immutable source tag. Never create a commit or include generated output.
  git('tag', tag, source)
  git('push', 'origin', `refs/tags/${tag}`)
  return tag
}
export async function publish(
  channel: ReleaseChannel,
  version: string,
  out: string,
  cwd = workspaceRoot,
): Promise<void> {
  const git = (...args: string[]) => execFileSync('git', args, { cwd, encoding: 'utf8' }).trim()
  const gh = (...args: string[]) => execFileSync('gh', args, { cwd, stdio: 'inherit' })
  const source = git('rev-parse', 'HEAD')
  await verifyReleaseAssets(out, channel, { version, commit: source })
  const tag = channel === 'stable' ? releaseTag(version) : 'main-build'
  const existing = spawnSync(
    'gh',
    ['release', 'view', tag, '--json', 'isDraft', '--jq', '.isDraft'],
    { cwd, encoding: 'utf8' },
  )
  if (channel === 'main') {
    if (git('ls-remote', 'origin', 'refs/heads/main').split(/\s+/)[0] !== source) {
      console.log('Main advanced; publication skipped')
      return
    }
    if (existing.status === 0) gh('release', 'delete', tag, '--yes')
    git('tag', '--force', tag, source)
    git('push', '--force', 'origin', `refs/tags/${tag}`)
  } else {
    if (git('ls-remote', 'origin', `refs/tags/${tag}`).split(/\s+/)[0] !== source)
      throw new Error('release tag differs from tested source')
    if (existing.status === 0 && existing.stdout.trim() !== 'true')
      throw new Error('published stable releases are immutable')
  }
  if (channel === 'main' || existing.status !== 0) {
    gh(
      'release',
      'create',
      tag,
      '--verify-tag',
      '--draft',
      '--title',
      channel === 'main' ? 'Main build' : `TokenInsights ${tag}`,
      ...(channel === 'main' ? ['--prerelease'] : ['--generate-notes']),
    )
  }
  gh(
    'release',
    'upload',
    tag,
    ...[...expectedArchives(channel), 'checksums.txt'].map((name) => join(out, name)),
    '--clobber',
  )
  gh(
    'release',
    'edit',
    tag,
    '--draft=false',
    `--prerelease=${channel === 'main'}`,
    `--latest=${channel === 'stable'}`,
  )
}
if (import.meta.main) {
  const [action, version, directory] = process.argv.slice(2)
  if (version === undefined)
    throw new Error('usage: publish-release.ts tag|stable|main <x.y.z|main.sha> [directory]')
  if (action === 'tag') console.log(prepareTag(version))
  else if (action === 'stable' || action === 'main')
    await publish(action, version, resolve(directory ?? 'dist'))
  else throw new Error(`unknown release action ${action}`)
}
