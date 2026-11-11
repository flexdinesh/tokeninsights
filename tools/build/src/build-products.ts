import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { cp, mkdir, mkdtemp, readFile, readdir, rm, stat, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import {
  buildDesktop,
  desktopIdentities,
  hostTarget,
  type DesktopProduct,
  type DesktopTarget,
} from './build-desktop.ts'
import { workspacePath, workspaceRoot } from './workspace.ts'

export type Product = 'cli' | DesktopProduct
export const products: readonly Product[] = ['cli', 'desktop', 'desktop-native']
export type ReleaseChannel = 'stable' | 'main' | 'edge'
// Edge ships only the native desktop experiment; stable and main never ship it.
export function channelProducts(channel: ReleaseChannel): readonly Product[] {
  return channel === 'edge' ? ['desktop-native'] : ['cli', 'desktop']
}
const stableVersion = /^(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$/
export function versionChannel(version: string): ReleaseChannel {
  if (/^main\.[0-9a-f]{12}$/.test(version)) return 'main'
  if (/^edge\.[0-9a-f]{12}$/.test(version)) return 'edge'
  if (stableVersion.test(version)) return 'stable'
  throw new Error(`invalid product version ${version}`)
}
function desktopProduct(product: Product): product is DesktopProduct {
  return product !== 'cli'
}
export interface BuildMetadata {
  version: string
  commit: string
  date: string
}
export function archiveRoot(product: Product, target: DesktopTarget): string {
  return `tokeninsights-${product}_${target.os}_${target.arch}`
}
export function archiveName(product: Product, target: DesktopTarget): string {
  return `${archiveRoot(product, target)}.tar.gz`
}
export function sourceMetadata(version: string, cwd = workspaceRoot): BuildMetadata {
  versionChannel(version)
  const git = (...args: string[]) => execFileSync('git', args, { cwd, encoding: 'utf8' }).trim()
  return {
    version,
    commit: git('rev-parse', 'HEAD'),
    date: git('show', '-s', '--format=%cI', 'HEAD'),
  }
}
export function linkerFlags(metadata: BuildMetadata): string {
  const module = 'github.com/flexdinesh/tokeninsights/packages/core/version'
  return `-s -w -X ${module}.Version=${metadata.version} -X ${module}.Commit=${metadata.commit} -X ${module}.Date=${metadata.date}`
}
async function executable(path: string): Promise<void> {
  const info = await stat(path)
  if (!info.isFile() || (info.mode & 0o111) === 0) throw new Error(`missing executable ${path}`)
}
export async function verifyProduct(
  root: string,
  product: Product,
  target: DesktopTarget,
): Promise<void> {
  const identity = desktopProduct(product) ? desktopIdentities[product] : undefined
  const binary = identity?.binary ?? 'tokeninsights'
  const bundle = identity !== undefined && target.os === 'darwin' ? identity.app : undefined
  const executableRoot = bundle === undefined ? root : join(root, bundle, 'Contents/MacOS')
  await executable(join(executableRoot, binary))
  if (product === 'desktop') await executable(join(executableRoot, 'tokeninsights'))
  await executable(join(executableRoot, 'tokeninsights-backend'))
  if (identity !== undefined && bundle !== undefined) {
    const plist = await readFile(join(root, bundle, 'Contents/Info.plist'), 'utf8')
    if (!plist.includes(`<string>${identity.identifier}</string>`))
      throw new Error('incompatible desktop identity')
  }
  await Promise.all(['LICENSE', 'build-info.json'].map((file) => stat(join(root, file))))
}
export async function packageProduct(
  stage: string,
  product: Product,
  target: DesktopTarget,
  out: string,
  metadata: BuildMetadata,
): Promise<string> {
  const root = join(stage, archiveRoot(product, target))
  await cp(workspacePath('LICENSE'), join(root, 'LICENSE'))
  await writeFile(
    join(root, 'build-info.json'),
    `${JSON.stringify({ format: 1, product, ...target, ...metadata }, null, 2)}\n`,
  )
  await writeFile(
    join(root, 'product.txt'),
    `tokeninsights-product-v1 ${product} ${target.os} ${target.arch}\n`,
  )
  await verifyProduct(root, product, target)
  const archive = join(out, archiveName(product, target))
  // Preserve framework links. Installers validate link targets and extraction containment.
  execFileSync('tar', ['-czf', archive, '-C', stage, archiveRoot(product, target)], {
    env: { ...process.env, COPYFILE_DISABLE: '1' },
    stdio: 'inherit',
  })
  return archive
}
function runBuildTool(command: string, args: string[]): void {
  execFileSync(command, args, {
    cwd: workspaceRoot,
    stdio: 'inherit',
    env: { ...process.env, CGO_ENABLED: '0' },
  })
}
export async function buildProducts(
  out: string,
  metadata: BuildMetadata,
  selected: readonly Product[] = channelProducts(versionChannel(metadata.version)),
): Promise<void> {
  const target = hostTarget()
  const temporary = await mkdtemp(join(tmpdir(), 'tokeninsights-products-'))
  await mkdir(out, { recursive: true })
  try {
    const binaries = join(temporary, 'bin')
    await mkdir(binaries)
    const commands = new Map([
      ['tokeninsights', './packages/cli/cmd/tokeninsights'],
      ['tokeninsights-backend', './packages/backend/cmd/tokeninsights-backend'],
    ])
    for (const [binary, command] of commands) {
      if (binary === 'tokeninsights' && !selected.includes('cli') && !selected.includes('desktop'))
        continue
      runBuildTool('go', [
        'build',
        '-trimpath',
        '-buildvcs=false',
        `-ldflags=${linkerFlags(metadata)}`,
        '-o',
        join(binaries, binary),
        command,
      ])
      if (binary !== 'tokeninsights-backend')
        execFileSync(join(binaries, binary), ['--version'], {
          env: { ...process.env, PATH: '' },
          stdio: 'inherit',
        })
    }
    for (const product of selected) {
      const root = join(temporary, archiveRoot(product, target))
      // Package construction is deliberately sequential, sharing one backend build.
      // eslint-disable-next-line no-await-in-loop
      await mkdir(root)
      const bundle =
        desktopProduct(product) && target.os === 'darwin'
          ? desktopIdentities[product].app
          : undefined
      const executableRoot = bundle === undefined ? root : join(root, bundle, 'Contents/MacOS')
      if (desktopProduct(product)) {
        // eslint-disable-next-line no-await-in-loop
        await buildDesktop(
          target,
          stableVersion.test(metadata.version) ? metadata.version : '0.0.0',
          linkerFlags(metadata),
          root,
          product,
        )
      }
      // Desktop exposes its bundled CLI; the native experiment never owns the command.
      if (product !== 'desktop-native')
        // eslint-disable-next-line no-await-in-loop
        await cp(join(binaries, 'tokeninsights'), join(executableRoot, 'tokeninsights'))
      // eslint-disable-next-line no-await-in-loop
      await cp(
        join(binaries, 'tokeninsights-backend'),
        join(executableRoot, 'tokeninsights-backend'),
      )
      if (bundle !== undefined)
        runBuildTool('codesign', ['--force', '--deep', '--sign', '-', join(root, bundle)])
      // eslint-disable-next-line no-await-in-loop
      await packageProduct(temporary, product, target, out, metadata)
    }
  } finally {
    await rm(temporary, { recursive: true, force: true })
  }
}
export async function writeChecksums(out: string): Promise<void> {
  const names = (await readdir(out)).filter((name) => name.endsWith('.tar.gz')).toSorted()
  const lines = await Promise.all(
    names.map(
      async (name) =>
        `${createHash('sha256')
          .update(await readFile(join(out, name)))
          .digest('hex')}  ${name}`,
    ),
  )
  if (lines.length === 0) throw new Error('no product archives')
  await writeFile(join(out, 'checksums.txt'), `${lines.join('\n')}\n`)
}
function isProduct(value: string): value is Product {
  return products.some((product) => product === value)
}
if (import.meta.main) {
  const [version, directory, ...names] = process.argv.slice(2)
  const selected = names.filter(isProduct)
  if (version === undefined || selected.length !== names.length)
    throw new Error(
      `usage: build-products.ts <x.y.z|main.sha|edge.sha> [output-directory] [${products.join('|')}...]`,
    )
  const out = resolve(workspaceRoot, directory ?? 'dist')
  const metadata = sourceMetadata(version)
  await buildProducts(out, metadata, selected.length === 0 ? undefined : selected)
  await writeChecksums(out)
}
