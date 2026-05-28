import { Worker } from "node:worker_threads"
import { createTokenInsightsLogger, errorFields } from "@tokeninsights/logger"
import type { TokenEventRow, TpsSampleRow, MessageInfoUpdate, TokenStorage, WriterResponse, WriterConfig } from "./types.ts"

const logger = createTokenInsightsLogger({ harness: "opencode-server" })

export function createTokenStorage(
  workerScriptUrl: URL,
  config: WriterConfig,
  onError: () => void,
): TokenStorage {
  let workerReady = false
  logger.debug("token worker create", { workerScriptUrl: workerScriptUrl.href })
  const worker = new Worker(workerScriptUrl)
  worker.on("message", (response: WriterResponse) => {
    if (response.type === "ready") {
      workerReady = true
      logger.debug("token worker ready")
      return
    }
    if (response.type === "flushed") {
      logger.debug("token worker flushed")
      return
    }
    if (response.type === "closed") {
      workerReady = false
      logger.debug("token worker closed")
      return
    }
    if (response.type === "error") {
      workerReady = false
      logger.error("token worker response error", { errorMessage: response.message })
      onError()
    }
  })
  worker.on("error", (err) => {
    workerReady = false
    logger.error("token worker emitted error", errorFields(err))
    onError()
  })
  worker.postMessage({ type: "init", dbPath: config.dbPath, retentionDays: config.retentionDays })

  return {
    flush(tokenRows, tpsRows, infoUpdates, toolRows) {
      if (!workerReady) {
        logger.debug("skip worker flush because worker is not ready")
        return
      }
      if (tokenRows.length === 0 && tpsRows.length === 0 && infoUpdates.length === 0 && toolRows.length === 0) return
      logger.debug("post worker flush", {
        tokenRows: tokenRows.length,
        tpsRows: tpsRows.length,
        infoUpdates: infoUpdates.length,
        toolRows: toolRows.length,
      })
      worker.postMessage({ type: "flush", tokenRows, tpsRows, infoUpdates, toolRows })
    },
    close() {
      logger.debug("post worker close")
      worker.postMessage({ type: "close" })
    },
  }
}
