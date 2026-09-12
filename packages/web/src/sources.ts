import { z } from 'zod'
import { bootstrapSchema, selectionSchema } from './contracts'
import type { Bootstrap } from './contracts'

const STORAGE_KEY = 'tokeninsights.sources.v1'
const STORAGE_VERSION = 1

const sourceSchema = z
  .object({
    baseUrl: z.string(),
    hostname: z.string().min(1),
    apiVersion: bootstrapSchema.shape.apiVersion.nullable(),
    serverVersion: z.string().min(1).nullable(),
    capabilities: bootstrapSchema.shape.capabilities,
    defaults: selectionSchema.nullable(),
  })
  .strict()

const persistedSourcesSchema = z
  .object({
    version: z.literal(STORAGE_VERSION),
    sources: z.array(sourceSchema),
    activeUrl: z.string(),
  })
  .strict()

export type Source = z.infer<typeof sourceSchema>

export type SourceMetadata = Pick<
  Bootstrap,
  'hostname' | 'apiVersion' | 'serverVersion' | 'capabilities' | 'defaults'
>

export interface SourceSnapshot {
  sources: Source[]
  activeUrl: string
}

export interface AddSourceResult {
  source: Source
  added: boolean
}

export interface SourceStore {
  getSnapshot(): SourceSnapshot
  subscribe(listener: () => void): () => void
  add(source: Source): AddSourceResult
  select(baseUrl: string): void
  remove(baseUrl: string): void
}

interface SourceStoreOptions {
  origin?: string
  storage?: Storage | null
}

function parseBaseUrl(input: string): URL {
  const value = input.trim()
  if (value === '') throw new TypeError('Source URL is required')

  const withProtocol =
    /^https?:\/\//i.test(value) || value.includes('://') ? value : `http://${value}`
  let url: URL
  try {
    url = new URL(withProtocol)
  } catch {
    throw new TypeError('Enter a valid HTTP or HTTPS source URL')
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    throw new TypeError('Source URL must use HTTP or HTTPS')
  }
  if (url.username !== '' || url.password !== '') {
    throw new TypeError('Source URL must not include credentials')
  }
  if (url.search !== '' || url.hash !== '') {
    throw new TypeError('Source URL must not include a query or fragment')
  }
  return url
}

export function normalizeBaseUrl(input: string): string {
  const url = parseBaseUrl(input)
  const path = url.pathname.replace(/\/+$/, '')
  return `${url.origin}${path}`
}

export function createSource(input: string, metadata?: SourceMetadata): Source {
  const baseUrl = normalizeBaseUrl(input)
  const url = new URL(baseUrl)
  return sourceSchema.parse({
    baseUrl,
    hostname: metadata?.hostname ?? url.hostname,
    apiVersion: metadata?.apiVersion ?? null,
    serverVersion: metadata?.serverVersion ?? null,
    capabilities: metadata?.capabilities ?? [],
    defaults: metadata?.defaults ?? null,
  })
}

function normalizeStoredSource(source: Source): Source | null {
  try {
    const base = createSource(source.baseUrl)
    return sourceSchema.parse({ ...source, baseUrl: base.baseUrl })
  } catch {
    return null
  }
}

function loadSnapshot(storage: Storage | undefined, local: Source): SourceSnapshot {
  if (!storage) return { sources: [local], activeUrl: local.baseUrl }
  try {
    const raw = storage.getItem(STORAGE_KEY)
    if (raw === null) return { sources: [local], activeUrl: local.baseUrl }
    const persisted: unknown = JSON.parse(raw)
    const parsed = persistedSourcesSchema.safeParse(persisted)
    if (!parsed.success) return { sources: [local], activeUrl: local.baseUrl }

    const sources: Source[] = []
    for (const stored of parsed.data.sources) {
      const source = normalizeStoredSource(stored)
      if (source && !sources.some((candidate) => candidate.baseUrl === source.baseUrl)) {
        sources.push(source)
      }
    }
    const localIndex = sources.findIndex((source) => source.baseUrl === local.baseUrl)
    if (localIndex === -1) sources.unshift(local)

    const activeUrl = sources.some((source) => source.baseUrl === parsed.data.activeUrl)
      ? parsed.data.activeUrl
      : local.baseUrl
    return { sources, activeUrl }
  } catch {
    return { sources: [local], activeUrl: local.baseUrl }
  }
}

function defaultStorage(): Storage | undefined {
  try {
    return window.localStorage
  } catch {
    return undefined
  }
}

function defaultOrigin(): string {
  return window.location.origin
}

export function createSourceStore(options: SourceStoreOptions = {}): SourceStore {
  const local = createSource(options.origin ?? defaultOrigin())
  const storage = options.storage === null ? undefined : (options.storage ?? defaultStorage())
  let snapshot = loadSnapshot(storage, local)
  const listeners = new Set<() => void>()

  const persist = () => {
    try {
      storage?.setItem(
        STORAGE_KEY,
        JSON.stringify({
          version: STORAGE_VERSION,
          sources: snapshot.sources,
          activeUrl: snapshot.activeUrl,
        }),
      )
    } catch {
      /* Source switching remains available in memory. */
    }
  }
  const publish = () => {
    persist()
    for (const listener of listeners) listener()
  }

  return {
    getSnapshot: () => snapshot,
    subscribe: (listener) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    add: (source) => {
      const base = createSource(source.baseUrl)
      const normalized = sourceSchema.parse({
        ...source,
        baseUrl: base.baseUrl,
      })
      const existing = snapshot.sources.find(
        (candidate) => candidate.baseUrl === normalized.baseUrl,
      )
      if (existing) {
        snapshot = {
          ...snapshot,
          sources: snapshot.sources.map((candidate) =>
            candidate.baseUrl === normalized.baseUrl ? normalized : candidate,
          ),
        }
        publish()
        return { source: normalized, added: false }
      }
      snapshot = { ...snapshot, sources: [...snapshot.sources, normalized] }
      publish()
      return { source: normalized, added: true }
    },
    select: (baseUrl) => {
      const normalized = normalizeBaseUrl(baseUrl)
      if (!snapshot.sources.some((source) => source.baseUrl === normalized)) {
        throw new Error('Source is not configured')
      }
      if (snapshot.activeUrl === normalized) return
      snapshot = { ...snapshot, activeUrl: normalized }
      publish()
    },
    remove: (baseUrl) => {
      const normalized = normalizeBaseUrl(baseUrl)
      if (normalized === local.baseUrl) throw new Error('Local source cannot be removed')
      const sources = snapshot.sources.filter((source) => source.baseUrl !== normalized)
      if (sources.length === snapshot.sources.length) return
      snapshot = {
        sources,
        activeUrl: snapshot.activeUrl === normalized ? local.baseUrl : snapshot.activeUrl,
      }
      publish()
    },
  }
}
