import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { cp, mkdir, mkdtemp, readFile, readdir, rm, symlink } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { promisify } from 'node:util'
import { fileURLToPath } from 'node:url'

const execute = promisify(execFile)
const workspace = fileURLToPath(new URL('../../../', import.meta.url))

async function fixture(
  formatter: boolean,
): Promise<{ parent: string; root: string; temporary: string }> {
  const parent = await mkdtemp(join(tmpdir(), 'tokeninsights-plugin-build-test-'))
  const root = join(parent, 'workspace spaces % #')
  const temporary = join(parent, 'temporary spaces % #')
  await Promise.all([
    mkdir(join(root, 'tools', 'build', 'src'), { recursive: true }),
    mkdir(join(root, 'packages'), { recursive: true }),
    mkdir(temporary),
  ])
  await Promise.all([
    cp(join(workspace, '.oxfmtrc.json'), join(root, '.oxfmtrc.json')),
    cp(join(workspace, 'package.json'), join(root, 'package.json')),
    ...['build-plugins.ts', 'plugin-runner.ts'].map((file) =>
      cp(join(workspace, 'tools', 'build', 'src', file), join(root, 'tools', 'build', 'src', file)),
    ),
    ...['pi', 'opencode'].map(async (harness) => {
      const destination = join(root, 'packages', `plugin-${harness}`)
      await mkdir(destination)
      await Promise.all(
        ['src', 'dist', 'package.json'].map((name) =>
          cp(join(workspace, 'packages', `plugin-${harness}`, name), join(destination, name), {
            recursive: true,
          }),
        ),
      )
    }),
    ...(formatter
      ? [symlink(join(workspace, 'node_modules'), join(root, 'node_modules'), 'dir')]
      : []),
  ])
  return { parent, root, temporary }
}

for (const harness of ['pi', 'opencode']) {
  void test(`${harness} real build/check supports encoded workspace and temporary paths`, async () => {
    const { parent, root, temporary } = await fixture(true)
    try {
      const script = join(root, 'tools', 'build', 'src', 'build-plugins.ts')
      const options = { cwd: root, env: { ...process.env, TMPDIR: temporary } }
      await execute(process.execPath, [script, harness], options)
      await Promise.all(
        ['index.js', 'completion.js', 'lifecycle.js', 'runner.js'].map(async (file) => {
          const [actual, expected] = await Promise.all([
            readFile(join(root, 'packages', `plugin-${harness}`, 'dist', file)),
            readFile(join(workspace, 'packages', `plugin-${harness}`, 'dist', file)),
          ])
          assert.deepEqual(actual, expected, `portable build changed committed ${file}`)
        }),
      )
      await execute(process.execPath, [script, harness, '--check'], options)
      assert.deepEqual(await readdir(temporary), [], 'check left temporary build artifacts')
    } finally {
      await rm(parent, { recursive: true, force: true })
    }
  })
}

void test('check removes temporary output when the formatter fails before comparison', async () => {
  const { parent, root, temporary } = await fixture(false)
  try {
    await assert.rejects(
      execute(
        process.execPath,
        [join(root, 'tools', 'build', 'src', 'build-plugins.ts'), 'pi', '--check'],
        {
          cwd: root,
          env: { ...process.env, TMPDIR: temporary },
        },
      ),
      /ENOENT/,
    )
    assert.deepEqual(await readdir(temporary), [], 'formatter failure leaked temporary output')
  } finally {
    await rm(parent, { recursive: true, force: true })
  }
})
