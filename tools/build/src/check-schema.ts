import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const SCHEMA_SQL_PATH = new URL('../../../schema/schema.sql', import.meta.url).pathname
const EMBEDDED_SCHEMA_SQL_PATH = new URL(
  '../../../packages/cli/internal/db/schema/schema.sql',
  import.meta.url,
).pathname
const SCHEMA_GO_PATH = new URL('../../../packages/cli/internal/db/schema.go', import.meta.url)
  .pathname

const SQL_KEYWORDS = new Set([
  'PRIMARY',
  'KEY',
  'AUTOINCREMENT',
  'NOT',
  'NULL',
  'DEFAULT',
  'CHECK',
  'UNIQUE',
  'INTEGER',
  'TEXT',
  'REAL',
  'IN',
  'REFERENCES',
  'FOREIGN',
  'ON',
  'DELETE',
  'UPDATE',
  'CASCADE',
  'RESTRICT',
  'SET',
  'IF',
  'EXISTS',
  'CREATE',
  'TABLE',
  'INDEX',
  'PRAGMA',
  'WAL',
  'BUSY_TIMEOUT',
  'USER_VERSION',
])

async function readText(path: string): Promise<string> {
  return await readFile(path, 'utf-8')
}

function extractSchemaSqlIdentifiers(sql: string): {
  tables: Set<string>
  columns: Set<string>
  indexes: Set<string>
  version: number
  applicationID: number
} {
  const tables = new Set<string>()
  const columns = new Set<string>()
  const indexes = new Set<string>()
  let version = 0
  let applicationID = 0
  let insideTable = false

  for (const rawLine of sql.split('\n')) {
    const line = rawLine.trim()

    const versionMatch = line.match(/^PRAGMA\s+user_version\s*=\s*(\d+)/i)
    if (versionMatch) {
      version = Number.parseInt(versionMatch[1], 10)
      continue
    }

    const applicationMatch = line.match(/^PRAGMA\s+application_id\s*=\s*(0x[0-9a-f]+|\d+)/i)
    if (applicationMatch) {
      applicationID = Number(applicationMatch[1])
      continue
    }

    const tableMatch = line.match(/^CREATE\s+TABLE\s+IF\s+NOT\s+EXISTS\s+(\S+)/i)
    if (tableMatch) {
      tables.add(tableMatch[1])
      insideTable = true
      continue
    }

    const indexMatch = line.match(/^CREATE\s+(?:UNIQUE\s+)?INDEX\s+IF\s+NOT\s+EXISTS\s+(\S+)/i)
    if (indexMatch) {
      indexes.add(indexMatch[1])
      continue
    }

    if (!insideTable) continue
    if (line.startsWith(');')) {
      insideTable = false
      continue
    }
    if (!line || line.startsWith('--') || line.startsWith('/*') || line.startsWith('*')) continue
    if (/^UNIQUE\s*\(/i.test(line)) continue
    if (/^CHECK\s*\(/i.test(line)) continue
    if (/^PRIMARY\s+KEY\s*\(/i.test(line)) continue
    if (/^FOREIGN\s+KEY\s*\(/i.test(line)) continue

    const firstToken = line.split(/\s+/)[0].replace(/,$/, '').replace(/\)$/, '')
    if (firstToken && !SQL_KEYWORDS.has(firstToken.toUpperCase())) {
      columns.add(firstToken)
    }
  }

  return { tables, columns, indexes, version, applicationID }
}

function extractGoConsts(go: string): { strings: Map<string, string>; ints: Map<string, number> } {
  const strings = new Map<string, string>()
  const ints = new Map<string, number>()
  for (const rawLine of go.split('\n')) {
    const line = rawLine.trim()
    if (!line.startsWith('const ')) continue
    const strMatch = line.match(/^const\s+(\w+)\s*=\s*"([^"]+)"/)
    if (strMatch) {
      strings.set(strMatch[1], strMatch[2])
      continue
    }
    const intMatch = line.match(/^const\s+(\w+)\s*=\s*(0x[0-9a-fA-F]+|\d+)/)
    if (intMatch) {
      ints.set(intMatch[1], Number(intMatch[2]))
    }
  }

  const constBlockRegex = /const\s+\(([^)]*)\)/gs
  let match: RegExpExecArray | null

  while ((match = constBlockRegex.exec(go)) !== null) {
    for (const rawLine of match[1].split('\n')) {
      const line = rawLine.trim()
      if (!line || line.startsWith('//')) continue

      const strMatch = line.match(/^(\w+)\s*=\s*"([^"]+)"/)
      if (strMatch) {
        strings.set(strMatch[1], strMatch[2])
        continue
      }

      const intMatch = line.match(/^(\w+)\s*=\s*(0x[0-9a-fA-F]+|\d+)/)
      if (intMatch) {
        ints.set(intMatch[1], Number(intMatch[2]))
      }
    }
  }

  return { strings, ints }
}

export function schemaContractMismatches(
  sql: string,
  embeddedSQL: string,
  go: string,
  applicationConstant: string,
  checkIdentifiers = true,
): string[] {
  const { tables, columns, indexes, version, applicationID } = extractSchemaSqlIdentifiers(sql)
  const { strings: goStrings, ints: goInts } = extractGoConsts(go)
  const allSqlIdentifiers = new Set([...tables, ...columns, ...indexes])
  const mismatches: string[] = []
  if (sql !== embeddedSQL) {
    mismatches.push('embedded Go schema copy differs from authoritative SQL')
  }
  if (checkIdentifiers) {
    for (const [constName, value] of goStrings) {
      if (!allSqlIdentifiers.has(value)) {
        mismatches.push(`Go const ${constName} = "${value}" not found in schema.sql`)
      }
    }

    for (const table of tables) {
      let found = false
      for (const [constName, value] of goStrings) {
        if (value === table && constName.startsWith('Table')) {
          found = true
          break
        }
      }
      if (!found) {
        mismatches.push(`Table "${table}" has no matching Go Table* constant`)
      }
    }
  }
  if (goInts.get('SupportedSchemaVersion') !== version) {
    mismatches.push(`SupportedSchemaVersion does not match schema user_version ${version}`)
  }
  if (applicationID === 0 || goInts.get(applicationConstant) !== applicationID) {
    mismatches.push(`${applicationConstant} does not match schema application_id ${applicationID}`)
  }
  return mismatches
}

async function main() {
  const [sql, embeddedSQL, go] = await Promise.all(
    [SCHEMA_SQL_PATH, EMBEDDED_SCHEMA_SQL_PATH, SCHEMA_GO_PATH].map(readText),
  )
  const [dataSQL, embeddedDataSQL, dataGo] = await Promise.all([
    readText(new URL('../../../schema/data.sql', import.meta.url).pathname),
    readText(
      new URL('../../../packages/cli/internal/datastore/schema/data.sql', import.meta.url).pathname,
    ),
    readText(
      new URL('../../../packages/cli/internal/datastore/store.go', import.meta.url).pathname,
    ),
  ])
  const [appSQL, embeddedAppSQL, appGo] = await Promise.all([
    readText(new URL('../../../schema/app.sql', import.meta.url).pathname),
    readText(
      new URL('../../../packages/cli/internal/appstore/schema/app.sql', import.meta.url).pathname,
    ),
    readText(new URL('../../../packages/cli/internal/appstore/store.go', import.meta.url).pathname),
  ])
  const [jobsSQL, embeddedJobsSQL, jobsGo] = await Promise.all([
    readText(new URL('../../../schema/jobs.sql', import.meta.url).pathname),
    readText(
      new URL('../../../packages/cli/internal/syncjob/schema/jobs.sql', import.meta.url).pathname,
    ),
    readText(new URL('../../../packages/cli/internal/syncjob/store.go', import.meta.url).pathname),
  ])
  const mismatches = [
    ...(jobsSQL === embeddedJobsSQL ? [] : ['jobs: embedded schema differs']),
    ...(extractGoConsts(jobsGo).ints.get('SchemaVersion') ===
    extractSchemaSqlIdentifiers(jobsSQL).version
      ? []
      : ['jobs: SchemaVersion mismatch']),
    ...(extractGoConsts(jobsGo).ints.get('ApplicationID') ===
    extractSchemaSqlIdentifiers(jobsSQL).applicationID
      ? []
      : ['jobs: ApplicationID mismatch']),

    ...(appSQL === embeddedAppSQL ? [] : ['application: embedded schema differs']),
    ...(extractGoConsts(appGo).ints.get('SchemaVersion') ===
    extractSchemaSqlIdentifiers(appSQL).version
      ? []
      : ['application: SchemaVersion mismatch']),
    ...(extractGoConsts(appGo).ints.get('ApplicationID') ===
    extractSchemaSqlIdentifiers(appSQL).applicationID
      ? []
      : ['application: ApplicationID mismatch']),

    ...(dataSQL === embeddedDataSQL ? [] : ['data: embedded DuckDB schema differs']),
    ...(extractGoConsts(dataGo).ints.get('SchemaVersion') ===
    Number(dataSQL.match(/version (\d+)/)?.[1])
      ? []
      : ['data: SchemaVersion mismatch']),
    ...schemaContractMismatches(sql, embeddedSQL, go, 'CollectorApplicationID').map(
      (message) => `collector: ${message}`,
    ),
  ]
  if (mismatches.length > 0) {
    for (const mismatch of mismatches) {
      console.error(mismatch)
    }
    process.exit(1)
  }

  console.log('schema contract OK')
}

if (process.argv[1] !== undefined && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await main()
}
