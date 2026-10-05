import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import { schemaContractMismatches } from '../src/check-schema.ts'

const sql = `CREATE TABLE IF NOT EXISTS publication_batches (
  batch_id TEXT PRIMARY KEY,
  request_bytes BLOB NOT NULL,
  receipt_bytes BLOB
);
CREATE UNIQUE INDEX IF NOT EXISTS publication_batches_pending_idx ON publication_batches(batch_id) WHERE receipt_bytes IS NULL;
PRAGMA application_id = 1414091587;
PRAGMA user_version = 15;
`
const go = `const (
 TablePublicationBatches = "publication_batches"
 ColBatchID = "batch_id"
 ColRequestBytes = "request_bytes"
 ColReceiptBytes = "receipt_bytes"
 IndexPending = "publication_batches_pending_idx"
)
const SupportedSchemaVersion = 15
const CollectorApplicationID = 0x54495343
`

void test('collector checks retain identifiers and recognize partial UNIQUE indexes and hex roles', () => {
  assert.deepEqual(schemaContractMismatches(sql, sql, go, 'CollectorApplicationID'), [])
  assert.match(
    schemaContractMismatches(
      sql,
      sql,
      go.replace('"request_bytes"', '"wrong_column"'),
      'CollectorApplicationID',
    ).join('\n'),
    /wrong_column/,
  )
  assert.match(
    schemaContractMismatches(
      sql,
      sql,
      go.replace('TablePublicationBatches', 'WrongName'),
      'CollectorApplicationID',
    ).join('\n'),
    /no matching Go Table/,
  )
})

void test('each role checks embedded bytes, independent version, and application identity', () => {
  const server = `PRAGMA user_version = 1;\nPRAGMA application_id = 1414091606;\n`
  const serverGo = `const SupportedSchemaVersion = 1\nconst ApplicationID = 0x54495356\n`
  assert.deepEqual(schemaContractMismatches(server, server, serverGo, 'ApplicationID', false), [])
  assert.match(
    schemaContractMismatches(server, server + '-- drift', serverGo, 'ApplicationID', false).join(
      '\n',
    ),
    /embedded Go schema copy/,
  )
  assert.match(
    schemaContractMismatches(
      server,
      server,
      serverGo.replace('Version = 1', 'Version = 2'),
      'ApplicationID',
      false,
    ).join('\n'),
    /user_version 1/,
  )
  assert.match(
    schemaContractMismatches(
      server,
      server,
      serverGo.replace('0x54495356', '0x54495343'),
      'ApplicationID',
      false,
    ).join('\n'),
    /application_id 1414091606/,
  )
  assert.match(
    schemaContractMismatches(
      '',
      '',
      'const SupportedSchemaVersion = 0\nconst ApplicationID = 0',
      'ApplicationID',
      false,
    ).join('\n'),
    /application_id 0/,
  )
})

void test('repository collector and server source and embedded contracts both agree', async () => {
  const paths = [
    '../../../schema/schema.sql',
    '../../../packages/cli/internal/db/schema/schema.sql',
    '../../../packages/cli/internal/db/schema.go',
    '../../../schema/server.sql',
    '../../../packages/cli/internal/serverstore/schema/server.sql',
    '../../../packages/cli/internal/serverstore/store.go',
  ]
  const [collector, embeddedCollector, collectorGo, server, embeddedServer, serverGo] =
    await Promise.all(paths.map((path) => readFile(new URL(path, import.meta.url), 'utf8')))
  assert.deepEqual(
    schemaContractMismatches(collector, embeddedCollector, collectorGo, 'CollectorApplicationID'),
    [],
  )
  assert.deepEqual(
    schemaContractMismatches(server, embeddedServer, serverGo, 'ApplicationID', false),
    [],
  )
})
