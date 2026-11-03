import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import {
  cp,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rename,
  rm,
  stat,
  writeFile,
} from 'node:fs/promises'
import { createRequire } from 'node:module'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { buildMygo, hostTarget, mygoApp, type DesktopTarget } from './build-desktop.ts'
import { workspacePath, workspaceRoot } from './workspace.ts'

export type Product = 'cli' | 'server' | 'mygo' | 'electron'
export const products: readonly Product[] = ['cli', 'server', 'mygo', 'electron']
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
  if (!/^(?:main\.[0-9a-f]{12}|(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*))$/.test(version))
    throw new Error(`invalid product version ${version}`)
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
  const desktop = product === 'mygo' || product === 'electron'
  const binary = product === 'cli' ? 'tokeninsights' : `tokeninsights-${product}`
  const app = product === 'mygo' ? mygoApp : 'TokenInsights Electron.app'
  const executableRoot =
    desktop && target.os === 'darwin' ? join(root, app, 'Contents/MacOS') : root
  await executable(join(executableRoot, binary))
  if (product !== 'server')
    await executable(
      join(
        product === 'electron'
          ? target.os === 'darwin'
            ? join(root, app, 'Contents/Resources')
            : join(root, 'resources')
          : executableRoot,
        'tokeninsights-backend',
      ),
    )
  if (desktop && target.os === 'darwin') {
    const plist = await readFile(join(root, app, 'Contents/Info.plist'), 'utf8')
    if (!plist.includes(`<string>com.flexdinesh.tokeninsights.${product}</string>`))
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
async function buildElectron(target: DesktopTarget, root: string): Promise<void> {
  // Electron installs its runtime explicitly; dependency installation provides
  // JavaScript/types only. Keep local, release and isolated product builds alike.
  execFileSync('pnpm', ['--filter', '@tokeninsights/desktop-electron', 'run', 'install:runtime'], {
    cwd: workspaceRoot,
    stdio: 'inherit',
  })
  execFileSync('pnpm', ['--filter', '@tokeninsights/desktop-electron', 'run', 'build'], {
    cwd: workspaceRoot,
    stdio: 'inherit',
  })
  const source = workspacePath('packages/desktop-electron')
  const require = createRequire(join(source, 'package.json'))
  const runtime = join(dirname(require.resolve('electron/package.json')), 'dist')
  let resources: string
  if (target.os === 'darwin') {
    const app = join(root, 'TokenInsights Electron.app')
    await cp(join(runtime, 'Electron.app'), app, { recursive: true, verbatimSymlinks: true })
    await rename(
      join(app, 'Contents/MacOS/Electron'),
      join(app, 'Contents/MacOS/tokeninsights-electron'),
    )
    const plist = join(app, 'Contents/Info.plist')
    const text = await readFile(plist, 'utf8')
    await writeFile(
      plist,
      text
        .replace(
          /(<key>CFBundleIdentifier<\/key>\s*<string>)[^<]+/,
          '$1com.flexdinesh.tokeninsights.electron',
        )
        .replace(/(<key>CFBundleExecutable<\/key>\s*<string>)[^<]+/, '$1tokeninsights-electron')
        .replace(/(<key>CFBundleName<\/key>\s*<string>)[^<]+/, '$1TokenInsights Electron')
        .replace(
          /<\/dict>\s*<\/plist>/,
          '<key>CFBundleURLTypes</key><array><dict><key>CFBundleURLSchemes</key><array><string>tokeninsights-electron</string></array></dict></array></dict></plist>',
        ),
    )
    resources = join(app, 'Contents/Resources')
  } else {
    await cp(runtime, root, { recursive: true, verbatimSymlinks: true })
    await rename(join(root, 'electron'), join(root, 'tokeninsights-electron'))
    resources = join(root, 'resources')
  }
  await rm(join(resources, 'default_app.asar'), { force: true })
  const app = join(resources, 'app')
  await mkdir(app, { recursive: true })
  await cp(join(source, 'dist'), join(app, 'dist'), { recursive: true })
  const config: unknown = JSON.parse(await readFile(join(source, 'package.json'), 'utf8'))
  if (
    typeof config !== 'object' ||
    config === null ||
    !('main' in config) ||
    typeof config.main !== 'string'
  )
    throw new Error('Electron package needs a main entrypoint')
  await writeFile(
    join(app, 'package.json'),
    `${JSON.stringify({ name: 'tokeninsights-electron', main: config.main, type: 'module' })}\n`,
  )
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
  selected: readonly Product[] = products,
): Promise<void> {
  const target = hostTarget()
  const temporary = await mkdtemp(join(tmpdir(), 'tokeninsights-products-'))
  await mkdir(out, { recursive: true })
  try {
    const binaries = join(temporary, 'bin')
    await mkdir(binaries)
    const commands = new Map([
      ['tokeninsights', './packages/cli/cmd/tokeninsights'],
      ['tokeninsights-server', './packages/server/cmd/tokeninsights-server'],
      ['tokeninsights-backend', './packages/backend/cmd/tokeninsights-backend'],
    ])
    for (const [binary, command] of commands) {
      if (binary === 'tokeninsights-server' && !selected.includes('server')) continue
      if (binary === 'tokeninsights' && !selected.includes('cli')) continue
      if (binary === 'tokeninsights-backend' && selected.every((product) => product === 'server'))
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
      if (product === 'mygo') {
        // eslint-disable-next-line no-await-in-loop
        await buildMygo(
          target,
          metadata.version.startsWith('main.') ? '0.0.0' : metadata.version,
          linkerFlags(metadata),
          root,
        )
      } else if (product === 'electron') {
        // eslint-disable-next-line no-await-in-loop
        await buildElectron(target, root)
      } else {
        const binary = product === 'cli' ? 'tokeninsights' : 'tokeninsights-server'
        // eslint-disable-next-line no-await-in-loop
        await cp(join(binaries, binary), join(root, binary))
      }
      if (product !== 'server') {
        const app = product === 'mygo' ? mygoApp : 'TokenInsights Electron.app'
        const dest =
          product === 'electron'
            ? target.os === 'darwin'
              ? join(root, app, 'Contents/Resources/tokeninsights-backend')
              : join(root, 'resources/tokeninsights-backend')
            : target.os === 'darwin' && product === 'mygo'
              ? join(root, app, 'Contents/MacOS/tokeninsights-backend')
              : join(root, 'tokeninsights-backend')
        // eslint-disable-next-line no-await-in-loop
        await cp(join(binaries, 'tokeninsights-backend'), dest)
      }
      if (target.os === 'darwin' && (product === 'mygo' || product === 'electron'))
        runBuildTool('codesign', [
          '--force',
          '--deep',
          '--sign',
          '-',
          join(root, product === 'mygo' ? mygoApp : 'TokenInsights Electron.app'),
        ])
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
if (import.meta.main) {
  const args = process.argv.slice(2)
  const version = args[0]
  if (version === undefined || args.length > 2)
    throw new Error('usage: build-products.ts <x.y.z|main.sha> [output-directory]')
  await buildProducts(resolve(workspaceRoot, args[1] ?? 'dist'), sourceMetadata(version))
  await writeChecksums(resolve(workspaceRoot, args[1] ?? 'dist'))
}
