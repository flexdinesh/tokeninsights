import assert from 'node:assert/strict'
import { mkdir, mkdtemp, rm, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

import { listFiles } from '../src/files.ts'

void test('lists regular files without following symbolic links or skipped names', async () => {
  const root = await mkdtemp(join(tmpdir(), 'tokeninsights-files-'))
  try {
    await mkdir(join(root, 'source', 'nested'), { recursive: true })
    await mkdir(join(root, 'node_modules', 'package'), { recursive: true })
    await writeFile(join(root, 'source', 'nested', 'file.txt'), 'file')
    await writeFile(join(root, 'top.txt'), 'top')
    await writeFile(join(root, 'node_modules', 'package', 'index.js'), 'skipped')
    await symlink(join(root, 'source'), join(root, 'linked'), 'dir')
    await symlink(join(root, 'top.txt'), join(root, 'linked.txt'))

    assert.deepEqual(await listFiles(root, new Set(['node_modules'])), [
      join('source', 'nested', 'file.txt'),
      'top.txt',
    ])
  } finally {
    await rm(root, { recursive: true, force: true })
  }
})
