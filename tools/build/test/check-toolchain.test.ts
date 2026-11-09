import assert from 'node:assert/strict'
import test from 'node:test'

import { toolchainMismatches, type ToolchainFiles } from '../src/check-toolchain.ts'

const files: ToolchainFiles = {
  miseToml: `min_version = "2026.10.18"

[settings]
go = "ignored"

[tools]
go = "1.27.3"
node = "26.12.1"
pnpm = "12.15.0"

[env]
node = "ignored"
`,
  dockerfile: `FROM debian:trixie-slim@sha256:abc AS base
FROM base AS toolchain
ENV MISE_VERSION=2026.10.18
RUN mise install
`,
  packageJson: '{"packageManager":"pnpm@12.15.0+sha512.abc"}',
  goMod: 'module example\n\ngo 1.27.0\n\ntoolchain go1.27.3\n',
}

void test('matching pins report no mismatches', () => {
  assert.deepEqual(toolchainMismatches(files), [])
})

void test('canonical Go version pins work when tidy omits a redundant toolchain directive', () => {
  assert.deepEqual(toolchainMismatches({ ...files, goMod: 'module example\n\ngo 1.27.3\n' }), [])
  assert.deepEqual(toolchainMismatches({ ...files, goMod: 'module example\n\ngo 1.27.2\n' }), [
    'go.mod toolchain: 1.27.2 differs from mise.toml 1.27.3',
  ])
})

void test('each copy that drifts from mise.toml is reported', () => {
  assert.deepEqual(
    toolchainMismatches({
      ...files,
      dockerfile: files.dockerfile.replace('MISE_VERSION=2026.10.18', 'MISE_VERSION=2026.10.1'),
      packageJson: '{"packageManager":"pnpm@12.14.0"}',
      goMod: 'module example\n\ngo 1.27.0\n\ntoolchain go1.27.2\n',
    }),
    [
      'check image mise: 2026.10.1 differs from mise.toml 2026.10.18',
      'package.json packageManager: 12.14.0 differs from mise.toml 12.15.0',
      'go.mod toolchain: 1.27.2 differs from mise.toml 1.27.3',
    ],
  )
})

void test('image tool pins outside mise.lock are rejected', () => {
  for (const pin of [
    'FROM golang:1.27.3-trixie AS go',
    'FROM node:26.12.1-trixie-slim AS web',
    'RUN npm install --global pnpm@12.15.0',
  ]) {
    assert.deepEqual(toolchainMismatches({ ...files, dockerfile: `${files.dockerfile}${pin}\n` }), [
      'check image Dockerfile: install Go, Node and pnpm only through mise.lock',
    ])
  }
})

void test('missing copies and pins are reported', () => {
  assert.deepEqual(
    toolchainMismatches({ ...files, packageJson: '{}', goMod: 'module example\n' }),
    [
      'package.json packageManager: version not found; expected 12.15.0',
      'go.mod toolchain: version not found; expected 1.27.3',
    ],
  )
  assert.deepEqual(
    toolchainMismatches({ ...files, miseToml: '[tools]\ngo = "1.27.3"\nnode = "26.12.1"\n' }),
    ['mise.toml: [tools] pnpm pin missing'],
  )
  assert.deepEqual(
    toolchainMismatches({ ...files, miseToml: files.miseToml.replace(/^min_version.*\n/, '') }),
    ['mise.toml: min_version missing'],
  )
  assert.deepEqual(toolchainMismatches({ ...files, dockerfile: 'FROM debian AS base\n' }), [
    'check image mise: version not found; expected 2026.10.18',
  ])
})
