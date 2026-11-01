import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import test from 'node:test'
import { workspacePath } from '../src/workspace.ts'

void test('push succeeds without validation tools available', () => {
  const result = spawnSync('/bin/sh', [workspacePath('.husky', 'pre-push')], {
    env: { PATH: '/nonexistent' },
    encoding: 'utf8',
  })
  assert.equal(result.status, 0, result.stderr)
  assert.equal(result.stdout, '')
})
