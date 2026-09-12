import assert from "node:assert/strict"
import { DatabaseSync } from "node:sqlite"
import { readFile, readdir } from "node:fs/promises"
import { join, relative } from "node:path"
import test from "node:test"

import { workspacePath } from "../src/workspace.mjs"

const fixtureDir = workspacePath(
  "packages",
  "cli",
  "testdata",
  "conformance",
  "sync-first-basic",
  "source",
)

const expectedFiles = [
  "claude-code/project/fixture-claude-session-one.jsonl",
  "claude-code/project/fixture-claude-session-two.jsonl",
  "codex/2026/01/01/rollout-2026-01-01T00-00-00-fixture-codex-session-one.jsonl",
  "codex/2026/01/02/rollout-2026-01-02T00-00-00-fixture-codex-session-two.jsonl",
  "opencode/source.sql",
  "pi/project/2026-01-01T00-00-00_fixture-pi-session-one.jsonl",
  "pi/project/2026-01-02T00-00-00_fixture-pi-session-two.jsonl",
]

const allowedFixtureStrings = new Set([
  "fixture-claude-message-one",
  "fixture-claude-message-two",
  "fixture-claude-request-one",
  "fixture-claude-request-two",
  "fixture-claude-session-one",
  "fixture-claude-session-two",
  "fixture-codex-session-one",
  "fixture-codex-session-two",
  "fixture-codex-turn-one",
  "fixture-codex-turn-two",
  "fixture-model-alpha",
  "fixture-model-beta",
  "fixture-model-delta",
  "fixture-model-epsilon",
  "fixture-model-eta",
  "fixture-model-gamma",
  "fixture-model-zeta",
  "fixture-oc-message-v1",
  "fixture-oc-message-v2",
  "fixture-oc-session-v1",
  "fixture-oc-session-v2",
  "fixture-pi-message-one",
  "fixture-pi-message-two",
  "fixture-pi-session-one",
  "fixture-pi-session-two",
  "fixture-provider-alpha",
  "fixture-provider-beta",
  "fixture-provider-delta",
  "fixture-provider-epsilon",
  "fixture-provider-gamma",
  "fixture-provider-zeta",
])

const allowedStructuralStrings = new Set([
  "assistant",
  "event_msg",
  "message",
  "session",
  "session_meta",
  "token_count",
  "turn_context",
])

const allowedNumbers = new Set([
  0,
  1,
  2,
  3,
  4,
  5,
  6,
  7,
  8,
  9,
  10,
  11,
  12,
  13,
  16,
  18,
  20,
  21,
  30,
  32,
  40,
  44,
  51,
  62,
  70,
  72,
  90,
  100,
  101,
  105,
  129,
  202,
  1767225601000,
  1767225602000,
  1767312001000,
  1767312002000,
])

test("shared fixture has only allowlisted public-safe source records", async () => {
  const paths = await listFiles(fixtureDir)
  const relativePaths = paths.map((path) => relative(fixtureDir, path)).sort()
  assert.deepEqual(relativePaths, expectedFiles)

  for (const path of paths) {
    const contents = await readFile(path, "utf8")
    assertPublicSafeText(`${relative(fixtureDir, path)}\n${contents}`)
    if (!path.endsWith(".jsonl")) {
      continue
    }

    const harness = relative(fixtureDir, path).split("/")[0]
    const lines = contents.trim().split("\n")
    assert.ok(lines.length > 0, `${path}: empty JSONL fixture`)
    for (const [index, line] of lines.entries()) {
      const record = JSON.parse(line)
      validateRecord(harness, record, `${path}:${index + 1}`)
      validateValues(record, `${path}:${index + 1}`)
    }
  }
})

test("OpenCode SQL materializes only allowlisted metadata rows", async () => {
  const sql = await readFile(join(fixtureDir, "opencode", "source.sql"), "utf8")
  assertPublicSafeText(sql)

  const database = new DatabaseSync(":memory:")
  try {
    database.exec(sql)
    const objects = database
      .prepare(
        "SELECT type, name FROM sqlite_master WHERE name NOT LIKE 'sqlite_autoindex_%' ORDER BY type, name",
      )
      .all()
      .map((row) => [row.type, row.name])
    assert.deepEqual(objects, [
      ["table", "message"],
      ["table", "session_message"],
    ])

    assert.deepEqual(tableColumns(database, "message"), [
      "id",
      "session_id",
      "time_created",
      "time_updated",
      "data",
    ])
    assert.deepEqual(tableColumns(database, "session_message"), [
      "id",
      "session_id",
      "type",
      "seq",
      "time_created",
      "time_updated",
      "data",
    ])

    const v1Rows = database
      .prepare("SELECT id, session_id, time_created, time_updated, data FROM message ORDER BY id")
      .all()
    assert.equal(v1Rows.length, 1)
    assert.deepEqual(Object.keys(v1Rows[0]), [
      "id",
      "session_id",
      "time_created",
      "time_updated",
      "data",
    ])
    validateValues(withoutData(v1Rows[0]), "opencode message row")
    validateOpenCodeV1(JSON.parse(v1Rows[0].data), "opencode message data")

    const v2Rows = database
      .prepare(
        "SELECT id, session_id, type, seq, time_created, time_updated, data FROM session_message ORDER BY id",
      )
      .all()
    assert.equal(v2Rows.length, 1)
    assert.deepEqual(Object.keys(v2Rows[0]), [
      "id",
      "session_id",
      "type",
      "seq",
      "time_created",
      "time_updated",
      "data",
    ])
    validateValues(withoutData(v2Rows[0]), "opencode session_message row")
    validateOpenCodeV2(JSON.parse(v2Rows[0].data), "opencode session_message data")
  } finally {
    database.close()
  }
})

test("fixture safety scan rejects representative sensitive data", () => {
  for (const value of [
    "owner@example.com",
    "/home/owner/private-project",
    "/Users/owner/private-project",
    "C:\\Users\\owner\\private-project",
    "sk-proj-1234567890abcdef",
    "ghp_1234567890abcdef",
    "-----BEGIN PRIVATE KEY-----",
    '{"prompt":"private conversation"}',
    '{"tool_output":"private result"}',
    '{"headers":{"authorization":"Bearer secret"}}',
  ]) {
    assert.throws(() => assertPublicSafeText(value), /unsafe fixture data/)
  }
})

function validateRecord(harness, record, context) {
  if (harness === "pi") {
    validatePi(record, context)
    return
  }
  if (harness === "codex") {
    validateCodex(record, context)
    return
  }
  if (harness === "claude-code") {
    validateClaudeCode(record, context)
    return
  }
  assert.fail(`${context}: unsupported harness ${harness}`)
}

function validatePi(record, context) {
  if (record.type === "session") {
    assertShape(record, ["type", "id"], [], context)
    return
  }
  assert.equal(record.type, "message", `${context}: unexpected Pi record type`)
  assertShape(record, ["type", "id", "message"], [], context)
  assertShape(record.message, ["role", "usage", "timestamp"], ["provider", "model"], context)
  assertShape(
    record.message.usage,
    ["input", "output", "cacheRead", "cacheWrite", "totalTokens"],
    [],
    context,
  )
}

function validateCodex(record, context) {
  if (record.type === "session_meta") {
    assertShape(record, ["type", "payload"], [], context)
    assertShape(record.payload, ["id", "model_provider"], [], context)
    return
  }
  if (record.type === "turn_context") {
    assertShape(record, ["type", "payload"], [], context)
    assertShape(record.payload, ["turn_id", "model"], [], context)
    return
  }
  assert.equal(record.type, "event_msg", `${context}: unexpected Codex record type`)
  assertShape(record, ["timestamp", "type", "payload"], [], context)
  assertShape(record.payload, ["type", "info"], [], context)
  assert.equal(record.payload.type, "token_count", `${context}: unexpected Codex event`)
  assertShape(
    record.payload.info,
    ["last_token_usage", "total_token_usage"],
    [],
    context,
  )
  for (const usage of [
    record.payload.info.last_token_usage,
    record.payload.info.total_token_usage,
  ]) {
    assertShape(
      usage,
      [
        "input_tokens",
        "cached_input_tokens",
        "output_tokens",
        "reasoning_output_tokens",
        "total_tokens",
      ],
      [],
      context,
    )
  }
}

function validateClaudeCode(record, context) {
  assertShape(
    record,
    ["type", "requestId", "timestamp", "sessionId", "message"],
    [],
    context,
  )
  assertShape(record.message, ["id", "role", "model", "usage"], ["provider"], context)
  assertShape(
    record.message.usage,
    [
      "input_tokens",
      "output_tokens",
      "cache_read_input_tokens",
      "cache_creation_input_tokens",
    ],
    [],
    context,
  )
}

function validateOpenCodeV1(data, context) {
  assertShape(data, ["role", "providerID", "modelID", "tokens", "time"], [], context)
  validateOpenCodeTokens(data.tokens, context)
  assertShape(data.time, ["created", "completed"], [], context)
  validateValues(data, context)
}

function validateOpenCodeV2(data, context) {
  assertShape(data, ["model", "tokens", "time"], [], context)
  assertShape(data.model, ["providerID", "id"], [], context)
  validateOpenCodeTokens(data.tokens, context)
  assertShape(data.time, ["created", "completed"], [], context)
  validateValues(data, context)
}

function validateOpenCodeTokens(tokens, context) {
  assertShape(tokens, ["input", "output", "reasoning", "cache"], [], context)
  assertShape(tokens.cache, ["read", "write"], [], context)
}

function assertShape(value, required, optional, context) {
  assert.equal(typeof value, "object", `${context}: expected object`)
  assert.notEqual(value, null, `${context}: expected object`)
  assert.ok(!Array.isArray(value), `${context}: expected object`)
  const actual = Object.keys(value).sort()
  const allowed = [...required, ...optional]
  assert.deepEqual(
    actual.filter((key) => !allowed.includes(key)),
    [],
    `${context}: unexpected fields`,
  )
  for (const key of required) {
    assert.ok(Object.hasOwn(value, key), `${context}: missing ${key}`)
  }
}

function validateValues(value, context) {
  if (value === null) {
    return
  }
  if (Array.isArray(value)) {
    for (const item of value) {
      validateValues(item, context)
    }
    return
  }
  if (typeof value === "object") {
    for (const [key, item] of Object.entries(value)) {
      validateValues(item, `${context}.${key}`)
    }
    return
  }
  if (typeof value === "number") {
    assert.ok(allowedNumbers.has(value), `${context}: unexpected numeric value ${value}`)
    return
  }
  if (typeof value === "string") {
    assertPublicSafeText(value)
    assert.ok(
      allowedFixtureStrings.has(value) ||
        allowedStructuralStrings.has(value) ||
        /^2026-01-0[12]T00:00:0[0-4]\.000Z$/.test(value),
      `${context}: unexpected string value ${value}`,
    )
    return
  }
  assert.fail(`${context}: unexpected ${typeof value} value`)
}

function assertPublicSafeText(value) {
  const forbidden = [
    /[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}/i,
    /(?:\/home\/|\/Users\/|[a-z]:\\Users\\|~\/)/i,
    /(?:sk-[a-z0-9-]{12,}|ghp_[a-z0-9]{12,}|github_pat_[a-z0-9_]{12,}|xox[baprs]-[a-z0-9-]{12,})/i,
    /-----BEGIN [A-Z ]*PRIVATE KEY-----/,
    /"(?:content|text|prompt|arguments|tool_input|tool_output|headers|signature|cost)"\s*:/i,
    /"authorization"\s*:/i,
  ]
  assert.ok(
    forbidden.every((pattern) => !pattern.test(value)),
    `unsafe fixture data: ${value}`,
  )
}

function tableColumns(database, table) {
  return database.prepare(`PRAGMA table_info(${table})`).all().map((row) => row.name)
}

function withoutData(row) {
  return Object.fromEntries(Object.entries(row).filter(([key]) => key !== "data"))
}

async function listFiles(dir) {
  const entries = await readdir(dir, { withFileTypes: true })
  const files = []
  for (const entry of entries) {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) {
      files.push(...(await listFiles(path)))
    } else if (entry.isFile()) {
      files.push(path)
    } else {
      assert.fail(`${path}: only regular files allowed`)
    }
  }
  return files
}
