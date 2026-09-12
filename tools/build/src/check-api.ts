import { mkdir, mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'

const ROOT = new URL('../../../', import.meta.url).pathname
const GO_OUTPUT = 'packages/cli/internal/server/api/types.gen.go'
const WEB_OUTPUT = 'packages/web/src/generated/api.ts'

function run(command: string, args: string[], cwd: string, env = process.env): void {
  const result = spawnSync(command, args, { cwd, encoding: 'utf8', env })
  if (result.error) throw result.error
  if (result.status !== 0) {
    throw new Error(result.stderr || result.stdout || `${command} failed`)
  }
}

async function sameFile(actual: string, generated: string): Promise<boolean> {
  const [actualContents, generatedContents] = await Promise.all([
    readFile(actual),
    readFile(generated),
  ])
  return actualContents.equals(generatedContents)
}

async function main(): Promise<void> {
  const temporary = await mkdtemp(join(tmpdir(), 'tokeninsights-api-'))
  try {
    const goDirectory = join(temporary, 'go')
    const webDirectory = join(temporary, 'web')
    await Promise.all([mkdir(goDirectory), mkdir(webDirectory)])
    const generatedGo = join(goDirectory, 'types.gen.go')
    const generatedWeb = join(webDirectory, 'api.ts')
    run(
      'go',
      [
        'tool',
        'oapi-codegen',
        '-generate',
        'models',
        '-package',
        'api',
        '-o',
        generatedGo,
        '../../docs/openapi.yaml',
      ],
      join(ROOT, 'packages/cli'),
    )
    run('pnpm', ['exec', 'orval', '--config', 'orval.config.ts'], ROOT, {
      ...process.env,
      TOKENINSIGHTS_API_OUTPUT: generatedWeb,
    })
    run('pnpm', ['exec', 'oxfmt', generatedWeb], ROOT)

    const mismatches: string[] = []
    if (!(await sameFile(join(ROOT, GO_OUTPUT), generatedGo))) mismatches.push(GO_OUTPUT)
    if (!(await sameFile(join(ROOT, WEB_OUTPUT), generatedWeb))) mismatches.push(WEB_OUTPUT)
    if (mismatches.length > 0) {
      throw new Error(`generated API files are stale: ${mismatches.join(', ')}`)
    }
  } finally {
    await rm(temporary, { force: true, recursive: true })
  }
}

await main()
