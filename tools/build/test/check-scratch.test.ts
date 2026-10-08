import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import test from 'node:test'

import { checkScratchFiles } from '../src/check-scratch.ts'
import { workspacePath } from '../src/workspace.ts'

for (const path of ['.scratch/plan.md', 'packages/example/.scratch/plan with\nnewline.md']) {
  void test(`ignored scratch stays local; force-add rejected: ${JSON.stringify(path)}`, async (t) => {
    const root = await mkdtemp(join(tmpdir(), 'tokeninsights-scratch-'))
    t.after(() => rm(root, { recursive: true, force: true }))
    const git = (...args: string[]): string =>
      execFileSync('git', args, { cwd: root, encoding: 'utf8' })
    git('init', '-q')
    await writeFile(join(root, '.gitignore'), await readFile(workspacePath('.gitignore')))
    await writeFile(join(root, 'README.md'), 'Durable docs\n')
    await writeFile(join(root, '.scratch-notes.md'), 'Durable docs\n')
    git('add', '.gitignore', 'README.md', '.scratch-notes.md')

    const local = join(root, path)
    await mkdir(dirname(local), { recursive: true })
    await writeFile(local, 'Local plan\n')
    assert.equal(
      execFileSync('git', ['check-ignore', '-z', '--stdin'], {
        cwd: root,
        encoding: 'utf8',
        input: `${path}\0`,
      }),
      `${path}\0`,
    )
    assert.doesNotThrow(() => checkScratchFiles(root))
    git('add', '--force', '--', path)
    assert.throws(() => checkScratchFiles(root), /Scratch files must remain local/)
    git('rm', '--cached', '--', path)
    assert.doesNotThrow(() => checkScratchFiles(root))
    assert.equal(await readFile(local, 'utf8'), 'Local plan\n')
  })
}

void test('staged removal of committed scratch passes while keeping the local file', async (t) => {
  const root = await mkdtemp(join(tmpdir(), 'tokeninsights-untrack-scratch-'))
  t.after(() => rm(root, { recursive: true, force: true }))
  const git = (...args: string[]): string =>
    execFileSync('git', args, { cwd: root, encoding: 'utf8' })
  git('init', '-q')
  await mkdir(join(root, '.scratch'))
  await writeFile(join(root, '.scratch', 'plan.md'), 'Local plan\n')
  git('add', '.scratch/plan.md')
  git(
    '-c',
    'user.name=Fixture',
    '-c',
    'user.email=fixture@example.invalid',
    'commit',
    '-qm',
    'fixture',
  )
  assert.throws(() => checkScratchFiles(root), /Scratch files must remain local/)
  git('rm', '--cached', '.scratch/plan.md')
  assert.doesNotThrow(() => checkScratchFiles(root))
  assert.equal(await readFile(join(root, '.scratch', 'plan.md'), 'utf8'), 'Local plan\n')
})

void test('scratch guard fails if Git cannot inspect the index', async (t) => {
  const root = await mkdtemp(join(tmpdir(), 'tokeninsights-no-git-'))
  t.after(() => rm(root, { recursive: true, force: true }))
  assert.throws(() => checkScratchFiles(root))
})
