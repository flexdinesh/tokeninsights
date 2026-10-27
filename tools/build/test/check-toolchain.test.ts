import assert from 'node:assert/strict'
import test from 'node:test'

import { toolchainMismatches, type ToolchainFiles } from '../src/check-toolchain.ts'

const files: ToolchainFiles = {
  miseToml: `[settings]
go = "ignored"

[tools]
go = "1.27.3"
node = "26.12.1"
pnpm = "12.15.0"

[env]
node = "ignored"
`,
  dockerfile: `FROM node:26.12.1-trixie-slim@sha256:abc AS web
RUN npm install --global pnpm@12.15.0
FROM golang:1.27.3-trixie@sha256:def AS build
`,
  packageJson: '{"packageManager":"pnpm@12.15.0+sha512.abc"}',
  goMod: 'module example\n\ngo 1.27.0\n\ntoolchain go1.27.3\n',
}

void test('matching pins report no mismatches', () => {
  assert.deepEqual(toolchainMismatches(files), [])
})

void test('each copy that drifts from mise.toml is reported', () => {
  assert.deepEqual(
    toolchainMismatches({
      ...files,
      dockerfile: files.dockerfile
        .replace('golang:1.27.3', 'golang:1.27.2')
        .replace('node:26.12.1', 'node:26.12.0')
        .replace('pnpm@12.15.0', 'pnpm@12.14.0'),
      packageJson: '{"packageManager":"pnpm@12.14.0"}',
      goMod: 'module example\n\ngo 1.27.0\n\ntoolchain go1.27.2\n',
    }),
    [
      'Dockerfile golang image: 1.27.2 differs from mise.toml 1.27.3',
      'Dockerfile node image: 26.12.0 differs from mise.toml 26.12.1',
      'Dockerfile pnpm install: 12.14.0 differs from mise.toml 12.15.0',
      'package.json packageManager: 12.14.0 differs from mise.toml 12.15.0',
      'packages/cli/go.mod toolchain: 1.27.2 differs from mise.toml 1.27.3',
    ],
  )
})

void test('missing copies and pins are reported', () => {
  assert.deepEqual(
    toolchainMismatches({ ...files, packageJson: '{}', goMod: 'module example\n\ngo 1.27.0\n' }),
    [
      'package.json packageManager: version not found; expected 12.15.0',
      'packages/cli/go.mod toolchain: version not found; expected 1.27.3',
    ],
  )
  assert.deepEqual(
    toolchainMismatches({ ...files, miseToml: '[tools]\ngo = "1.27.3"\nnode = "26.12.1"\n' }),
    ['mise.toml: [tools] pnpm pin missing'],
  )
})
