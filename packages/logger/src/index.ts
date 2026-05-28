import { appendFile, mkdir, readdir, unlink } from "node:fs/promises"
import { join } from "node:path"

export type LogLevel = "debug" | "info" | "warn" | "error"
export type LogFormat = "logfmt" | "jsonl"
export type LogValue = string | number | boolean | null | undefined
export type LogFields = Record<string, LogValue>

export const LOG_FORMAT: LogFormat = "logfmt"

const LOG_FORMATTERS: Record<LogFormat, (input: { ts: string; level: LogLevel; harness: string; message: string; fields?: LogFields }) => string> = {
  logfmt: formatLogfmt,
  jsonl: formatJsonl,
}

const LOG_RETENTION_DAYS = 2
const TOKENINSIGHTS_STATE_DIR = "tokeninsights"
const LOG_DIR_NAME = "logs"
const LOG_FILE_PATTERN = /^([A-Za-z0-9._-]+)-(\d{2})-(\d{2})-(\d{2})\.log$/

export type TokenInsightsLogger = {
  debug: (message: string, fields?: LogFields) => void
  info: (message: string, fields?: LogFields) => void
  warn: (message: string, fields?: LogFields) => void
  error: (message: string, fields?: LogFields) => void
  flush: () => Promise<void>
  close: () => Promise<void>
}

export type TokenInsightsLoggerConfig = {
  harness: string
}

function pad2(value: number) {
  return String(value).padStart(2, "0")
}

function dayKey(date: Date) {
  return `${pad2(date.getFullYear() % 100)}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
}

function logDir() {
  const xdgStateHome = process.env.XDG_STATE_HOME?.trim()
  if (xdgStateHome && xdgStateHome.length > 0) return join(xdgStateHome, TOKENINSIGHTS_STATE_DIR, LOG_DIR_NAME)

  const home = process.env.HOME?.trim()
  if (home && home.length > 0) return join(home, ".local", "state", TOKENINSIGHTS_STATE_DIR, LOG_DIR_NAME)

  return join(process.cwd(), ".tokeninsights-state", LOG_DIR_NAME)
}

function logPath(harness: string, date: Date) {
  return join(logDir(), `${harness}-${dayKey(date)}.log`)
}

function debugEnabled() {
  return process.env.TOKENINSIGHTS_DEBUG === "1"
}

function sanitizeKey(key: string) {
  const cleaned = key.replace(/[^A-Za-z0-9_.-]/g, "_")
  return cleaned.length > 0 ? cleaned : "field"
}

function logfmtValue(value: LogValue) {
  if (value === undefined) return undefined
  if (value === null) return "null"
  if (typeof value === "number") return Number.isFinite(value) ? String(value) : "null"
  if (typeof value === "boolean") return value ? "true" : "false"
  const escaped = value.replace(/\\/g, "\\\\").replace(/"/g, "\\\"").replace(/\n/g, "\\n").replace(/\r/g, "\\r")
  if (escaped.length === 0) return "\"\""
  return /^[^\s="]+$/.test(escaped) ? escaped : `"${escaped}"`
}

function formatLogfmt(input: { ts: string; level: LogLevel; harness: string; message: string; fields?: LogFields }) {
  const parts = [
    `ts=${logfmtValue(input.ts)}`,
    `level=${input.level}`,
    `harness=${logfmtValue(input.harness)}`,
    `msg=${logfmtValue(input.message)}`,
  ]
  if (input.fields) {
    for (const [key, value] of Object.entries(input.fields)) {
      const formatted = logfmtValue(value)
      if (formatted !== undefined) parts.push(`${sanitizeKey(key)}=${formatted}`)
    }
  }
  return `${parts.join(" ")}\n`
}

function formatJsonl(input: { ts: string; level: LogLevel; harness: string; message: string; fields?: LogFields }) {
  return `${JSON.stringify({ ts: input.ts, level: input.level, harness: input.harness, msg: input.message, fields: input.fields ?? {} })}\n`
}

function formatLine(input: { ts: string; level: LogLevel; harness: string; message: string; fields?: LogFields }) {
  return LOG_FORMATTERS[LOG_FORMAT](input)
}

function parsedLogDate(name: string): Date | undefined {
  const match = LOG_FILE_PATTERN.exec(name)
  if (!match) return undefined
  const year = Number(match[2]) + 2000
  const month = Number(match[3])
  const day = Number(match[4])
  if (!Number.isInteger(year) || !Number.isInteger(month) || !Number.isInteger(day)) return undefined
  if (month < 1 || month > 12 || day < 1 || day > 31) return undefined
  return new Date(year, month - 1, day)
}

function retentionCutoff(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate() - LOG_RETENTION_DAYS)
}

async function pruneOldLogs(now: Date) {
  try {
    const dir = logDir()
    const entries = await readdir(dir, { withFileTypes: true })
    const cutoff = retentionCutoff(now).getTime()
    for (const entry of entries) {
      if (!entry.isFile()) continue
      const parsed = parsedLogDate(entry.name)
      if (!parsed) continue
      if (parsed.getTime() < cutoff) {
        await unlink(join(dir, entry.name))
      }
    }
  } catch {
    // Logging is best-effort only.
  }
}

export function errorFields(error: unknown): LogFields {
  if (error instanceof Error) {
    return {
      errorName: error.name,
      errorMessage: error.message,
    }
  }
  return { errorMessage: String(error) }
}

export function createTokenInsightsLogger(config: TokenInsightsLoggerConfig): TokenInsightsLogger {
  let queue = Promise.resolve()
  let preparedDay = ""
  let prunedDay = ""

  const enqueue = (work: () => Promise<void>) => {
    queue = queue.then(work, work).catch(() => {
      // Logging is best-effort only.
    })
  }

  const write = (level: LogLevel, message: string, fields?: LogFields) => {
    if (level === "debug" && !debugEnabled()) return

    const now = new Date()
    const currentDay = dayKey(now)
    const line = formatLine({ ts: now.toISOString(), level, harness: config.harness, message, fields })
    const path = logPath(config.harness, now)

    enqueue(async () => {
      try {
        if (preparedDay !== currentDay) {
          await mkdir(logDir(), { recursive: true })
          preparedDay = currentDay
        }
        if (prunedDay !== currentDay) {
          await pruneOldLogs(now)
          prunedDay = currentDay
        }
        await appendFile(path, line, "utf8")
      } catch {
        // Logging is best-effort only.
      }
    })
  }

  const flush = async () => {
    await queue.catch(() => {
      // Logging is best-effort only.
    })
  }

  return {
    debug(message, fields) {
      write("debug", message, fields)
    },
    info(message, fields) {
      write("info", message, fields)
    },
    warn(message, fields) {
      write("warn", message, fields)
    },
    error(message, fields) {
      write("error", message, fields)
    },
    flush,
    close: flush,
  }
}
