import { mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises'
import { execFile } from 'node:child_process'
import { stripTypeScriptTypes } from 'node:module'
import { promisify } from 'node:util'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const harness = process.argv[2]
if (harness !== 'pi' && harness !== 'opencode')
  throw new Error('Expected plugin harness pi or opencode')

const runtimePackage = '@tokeninsights/plugin-runtime'
const runtimeOutput = 'runner.js'
const localModule = /^\.\/[\w-]+\.ts$/
// Static import/export declarations, including side-effect imports and
// multi-line specifier lists. Type-only declarations are already stripped.
const moduleSpecifier = /^(\s*(?:import|export)\s(?:[^;'"]*?\sfrom\s*)?)(['"])([^'"\n]+)\2/gm

const packageRoot = new URL(`../../../packages/plugin-${harness}/`, import.meta.url)
const runtimeSource = new URL('../plugin-runtime/src/plugin-runner.ts', packageRoot)
const check = process.argv[3] === '--check'
const temporary = check ? await mkdtemp(join(tmpdir(), 'tokeninsights-plugin-build-')) : undefined
const output =
  temporary === undefined ? new URL('dist/', packageRoot) : pathToFileURL(`${temporary}/`)

// Dist must load without the workspace: local modules become sibling .js files
// and the shared runtime is copied in as runner.js.
function rewriteSpecifier(specifier: string, file: string): string {
  if (specifier === runtimePackage) return `./${runtimeOutput}`
  if (localModule.test(specifier)) return specifier.replace(/\.ts$/, '.js')
  if (specifier.startsWith('.'))
    throw new Error(`Plugin ${harness} ${file} imports ${specifier} outside its sources`)
  return specifier
}

try {
  const sources = (await readdir(new URL('src/', packageRoot)))
    .filter((name) => name.endsWith('.ts') && !name.endsWith('.d.ts'))
    .toSorted()
  const files = [
    ...sources.map((name) => ({
      input: new URL(`src/${name}`, packageRoot),
      name: name.replace(/\.ts$/, '.js'),
    })),
    { input: runtimeSource, name: runtimeOutput },
  ]
  if (temporary === undefined) await rm(output, { recursive: true, force: true })
  await mkdir(output, { recursive: true })
  await Promise.all(
    files.map(async (file) => {
      const source = await readFile(file.input, 'utf8')
      const javascript = stripTypeScriptTypes(source, { mode: 'strip' }).replace(
        moduleSpecifier,
        (_match, prefix: string, quote: string, specifier: string) =>
          `${prefix}${quote}${rewriteSpecifier(specifier, file.name)}${quote}`,
      )
      await writeFile(new URL(file.name, output), javascript)
    }),
  )
  await promisify(execFile)(
    fileURLToPath(new URL('../../../node_modules/.bin/oxfmt', import.meta.url)),
    [fileURLToPath(output)],
  )
  if (temporary !== undefined) {
    const expected = files.map((file) => file.name).toSorted()
    const committed = (await readdir(new URL('dist/', packageRoot))).toSorted()
    if (expected.join('\n') !== committed.join('\n'))
      throw new Error(
        `Plugin ${harness} artifacts ${committed.join(', ')} differ from ${expected.join(', ')}; run pnpm run build:plugins`,
      )
    await Promise.all(
      files.map(async (file) => {
        const [built, current] = await Promise.all([
          readFile(new URL(file.name, output)),
          readFile(new URL(`dist/${file.name}`, packageRoot)),
        ])
        if (!built.equals(current))
          throw new Error(
            `Plugin ${harness} artifact ${file.name} is stale; run pnpm run build:plugins`,
          )
      }),
    )
  }
} finally {
  if (temporary !== undefined) await rm(temporary, { recursive: true, force: true })
}
