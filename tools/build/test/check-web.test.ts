import assert from 'node:assert/strict'
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

import { webAssetDifferences } from '../src/check-web.ts'

void test('web asset comparison reports content and file-set drift', async () => {
  const root = await mkdtemp(join(tmpdir(), 'tokeninsights-web-assets-'))
  const expected = join(root, 'expected')
  const actual = join(root, 'actual')
  try {
    await Promise.all([mkdir(expected), mkdir(actual)])
    await Promise.all([
      writeFile(join(expected, 'same.txt'), 'same'),
      writeFile(join(actual, 'same.txt'), 'same'),
      writeFile(join(expected, 'changed.txt'), 'expected'),
      writeFile(join(actual, 'changed.txt'), 'actual'),
      writeFile(join(expected, 'missing.txt'), 'missing'),
      writeFile(join(expected, 'mockServiceWorker.js'), 'development only'),
      writeFile(join(actual, 'unexpected.txt'), 'unexpected'),
    ])

    assert.deepEqual(await webAssetDifferences(expected, actual), [
      'changed changed.txt',
      'missing missing.txt',
      'unexpected unexpected.txt',
    ])
  } finally {
    await rm(root, { force: true, recursive: true })
  }
})
