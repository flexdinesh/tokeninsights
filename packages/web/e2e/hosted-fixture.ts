import { request, type Server } from 'node:http'
import { createServer } from 'node:https'
import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import { z } from 'zod'
import { RawCapabilitiesV3, UsageResponseV2 } from '../src/generated/api.ts'

export const userSchema = z.object({ userId: z.string(), datasetId: z.string() })
export const tokenSchema = z.object({ tokenId: z.string(), token: z.string() })

export async function adminRequest<T>(
  socket: string,
  body: Record<string, unknown>,
  schema: z.ZodType<T>,
): Promise<T> {
  return new Promise((resolveRequest, reject) => {
    const outgoing = request(
      {
        socketPath: socket,
        path: '/control/v1/accounts',
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      },
      (incoming) => {
        let response = ''
        incoming.setEncoding('utf8')
        incoming.on('data', (chunk: string) => {
          response += chunk
        })
        incoming.on('end', () => {
          try {
            if (incoming.statusCode !== 200)
              throw new Error('Hosted fixture admin operation failed')
            resolveRequest(schema.parse(JSON.parse(response)))
          } catch (error) {
            reject(error)
          }
        })
        incoming.on('error', reject)
      },
    )
    outgoing.on('error', reject)
    outgoing.setTimeout(5_000, () => outgoing.destroy(new Error('Admin request timed out')))
    outgoing.end(JSON.stringify(body))
  })
}

export async function startHostedProxy(backendURL: () => string): Promise<Server> {
  const [key, cert] = await Promise.all([
    readFile(resolve('e2e/fixtures/hosted-key.pem')),
    readFile(resolve('e2e/fixtures/hosted-cert.pem')),
  ])
  const proxy = createServer({ key, cert }, (incoming, response) => {
    const backend = backendURL()
    if (!backend) {
      response.writeHead(503).end()
      return
    }
    const upstream = request(
      new URL(incoming.url ?? '/', backend),
      {
        method: incoming.method,
        headers: incoming.headers,
      },
      (upstreamResponse) => {
        response.writeHead(upstreamResponse.statusCode ?? 502, upstreamResponse.headers)
        upstreamResponse.pipe(response)
      },
    )
    upstream.on('error', () => {
      response.writeHead(502)
      response.end()
    })
    upstream.setTimeout(5_000, () => upstream.destroy(new Error('Proxy request timed out')))
    response.on('close', () => upstream.destroy())
    incoming.pipe(upstream)
  })
  await new Promise<void>((resolveListening, reject) => {
    proxy.once('error', reject)
    proxy.listen(0, '127.0.0.1', resolveListening)
  })
  return proxy
}

export async function seedHostedUser(
  hostedBackend: string,
  token: string,
  input: number,
  signal: AbortSignal,
): Promise<void> {
  const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }
  const capabilitiesResponse = await fetch(hostedBackend + '/api/v3/ingestion/capabilities', {
    headers,
    signal: AbortSignal.any([signal, AbortSignal.timeout(5_000)]),
  })
  const capabilities = RawCapabilitiesV3.parse(await capabilitiesResponse.json())
  const batch = {
    protocolVersion: 3,
    extractorVersion: 1,
    databaseId: capabilities.databaseId,
    datasetId: capabilities.datasetId,
    streamId: 'same-stream',
    batchId: 'same-batch',
    fromSequence: 1,
    toSequence: 1,
    entries: [
      {
        sequence: 1,
        record: {
          harness: 'pi',
          format: 'pi-jsonl',
          sourceId: 'same-source',
          lineage: 'same-lineage',
          ordinal: 1,
          data: {
            type: 'message',
            id: 'same-message',
            message: {
              role: 'assistant',
              timestamp: Date.now(),
              provider: 'synthetic-provider',
              model: 'synthetic-model',
              usage: { input, output: 20 },
            },
          },
          context: [{ ordinal: 0, data: { type: 'session', id: 'same-session' } }],
        },
      },
    ],
  }
  const acceptance = await fetch(hostedBackend + '/api/v3/ingestion/batches', {
    signal: AbortSignal.any([signal, AbortSignal.timeout(5_000)]),
    method: 'POST',
    headers,
    body: JSON.stringify(batch),
  })
  if (acceptance.status !== 202) throw new Error('Hosted fixture evidence acceptance failed')
  const deadline = Date.now() + 30_000
  for (;;) {
    const response = await fetch(hostedBackend + '/api/v2/usage?period=all', {
      headers,
      signal: AbortSignal.any([signal, AbortSignal.timeout(5_000)]),
    })
    const usage = UsageResponseV2.parse(await response.json())
    if (usage.pending === 0 && usage.summary.total === input + 20) return
    if (Date.now() >= deadline) throw new Error('Hosted fixture processing timed out')
    await new Promise<void>((resolveWait) => setTimeout(resolveWait, 50))
  }
}
