import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { cp, mkdir, mkdtemp, readFile, rm, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'

const execute = promisify(execFile)
const workspace = fileURLToPath(new URL('../../../', import.meta.url))

void test('API verification rejects generated drift without repairing either output', async () => {
  const root = await mkdtemp(join(tmpdir(), 'tokeninsights-api-check-'))
  const outputs = [
    'packages/cli/internal/server/api/types.gen.go',
    'packages/web/src/generated/api.ts',
  ]
  try {
    await Promise.all(
      [
        'tools/build/src/check-api.ts',
        'package.json',
        '.oxfmtrc.json',
        'orval.config.ts',
        'docs/openapi.yaml',
        'packages/cli/go.mod',
        'packages/cli/go.sum',
        'packages/cli/internal/server/api/oapi-codegen.yaml',
        ...outputs,
      ].map(async (path) => {
        await mkdir(dirname(join(root, path)), { recursive: true })
        await cp(join(workspace, path), join(root, path))
      }),
    )
    await symlink(join(workspace, 'node_modules'), join(root, 'node_modules'), 'dir')
    const script = join(root, 'tools/build/src/check-api.ts')
    await execute(process.execPath, [script], { cwd: root })
    await Promise.all(
      outputs.map((path) => writeFile(join(root, path), '// stale generated output\n')),
    )
    await assert.rejects(
      execute(process.execPath, [script], { cwd: root }),
      /generated API files are stale: .*types.gen.go, .*api.ts/,
    )
    await Promise.all(
      outputs.map(async (path) => {
        assert.equal(await readFile(join(root, path), 'utf8'), '// stale generated output\n')
      }),
    )
  } finally {
    await rm(root, { recursive: true, force: true })
  }
})
