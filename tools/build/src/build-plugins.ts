import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { execFile } from 'node:child_process'
import { stripTypeScriptTypes } from 'node:module'
import { promisify } from 'node:util'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const harness = process.argv[2]
if (harness !== 'pi' && harness !== 'opencode')
  throw new Error('Expected plugin harness pi or opencode')

const packageRoot = new URL(`../../../packages/plugin-${harness}/`, import.meta.url)
const check = process.argv[3] === '--check'
const temporary = check ? await mkdtemp(join(tmpdir(), 'tokeninsights-plugin-build-')) : undefined
const output =
  temporary === undefined ? new URL('dist/', packageRoot) : pathToFileURL(`${temporary}/`)
try {
  await mkdir(output, { recursive: true })
  const files = [
    { input: new URL('src/index.ts', packageRoot), name: 'index.js' },
    { input: new URL('src/completion.ts', packageRoot), name: 'completion.js' },
    { input: new URL('src/lifecycle.ts', packageRoot), name: 'lifecycle.js' },
    { input: new URL('plugin-runner.ts', import.meta.url), name: 'runner.js' },
  ]
  await Promise.all(
    files.map(async (file) => {
      const source = await readFile(file.input, 'utf8')
      const javascript = stripTypeScriptTypes(source, { mode: 'strip' })
        .replaceAll('../../../tools/build/src/plugin-runner.ts', './runner.js')
        .replaceAll('./completion.ts', './completion.js')
        .replaceAll('./lifecycle.ts', './lifecycle.js')
      await writeFile(new URL(file.name, output), javascript)
    }),
  )
  await promisify(execFile)(
    fileURLToPath(new URL('../../../node_modules/.bin/oxfmt', import.meta.url)),
    [fileURLToPath(output)],
  )
  if (temporary !== undefined) {
    await Promise.all(
      files.map(async (file) => {
        const [built, committed] = await Promise.all([
          readFile(new URL(file.name, output)),
          readFile(new URL(`dist/${file.name}`, packageRoot)),
        ])
        if (!built.equals(committed))
          throw new Error(
            `Plugin ${harness} artifact ${file.name} is stale; run pnpm run build:plugins`,
          )
      }),
    )
  }
} finally {
  if (temporary !== undefined) await rm(temporary, { recursive: true, force: true })
}
