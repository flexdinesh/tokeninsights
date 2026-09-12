import { describe, expect, it, vi } from 'vitest'
import type { Selection } from './contracts'
import { createSource, createSourceStore, normalizeBaseUrl, type SourceMetadata } from './sources'

const defaults: Selection = {
  period: 'week',
  bucket: 'day',
  from: '',
  to: '',
  providers: [],
  models: [],
  harnesses: [],
  sessions: [],
}

const metadata: SourceMetadata = {
  hostname: 'remote-workstation',
  apiVersion: 'v1',
  serverVersion: '1.2.3',
  capabilities: ['usage', 'sync'],
  defaults,
}

class MemoryStorage implements Storage {
  private values = new Map<string, string>()

  get length() {
    return this.values.size
  }

  clear() {
    this.values.clear()
  }

  getItem(key: string) {
    return this.values.get(key) ?? null
  }

  key(index: number) {
    return [...this.values.keys()][index] ?? null
  }

  removeItem(key: string) {
    this.values.delete(key)
  }

  setItem(key: string, value: string) {
    this.values.set(key, value)
  }
}

class ThrowingStorage implements Storage {
  get length(): number {
    throw new Error('storage unavailable')
  }

  clear(): void {
    throw new Error('storage unavailable')
  }

  getItem(): string | null {
    throw new Error('storage unavailable')
  }

  key(): string | null {
    throw new Error('storage unavailable')
  }

  removeItem(): void {
    throw new Error('storage unavailable')
  }

  setItem(): void {
    throw new Error('storage unavailable')
  }
}

describe('source URL normalization', () => {
  it('normalizes bare addresses and HTTP URLs', () => {
    expect(normalizeBaseUrl(' localhost:8765 ')).toBe('http://localhost:8765')
    expect(normalizeBaseUrl('HTTP://Example.COM:80/proxy/')).toBe('http://example.com/proxy')
    expect(normalizeBaseUrl('https://[::1]:8765/')).toBe('https://[::1]:8765')
  })

  it.each([
    '',
    'ftp://example.com',
    'https://user:secret@example.com',
    'https://example.com?token=secret',
    'https://example.com/#section',
  ])('rejects invalid base URL %j', (value) => {
    expect(() => normalizeBaseUrl(value)).toThrow(/Source URL|Enter a valid/)
  })

  it('uses the validated instance hostname and retains its metadata', () => {
    expect(createSource('https://Example.com:8443', metadata)).toEqual({
      baseUrl: 'https://example.com:8443',
      ...metadata,
    })
  })
})

describe('source persistence', () => {
  it('seeds the page origin and persists sources and active selection', () => {
    const storage = new MemoryStorage()
    const store = createSourceStore({ origin: 'http://localhost:8765', storage })
    const remote = createSource('http://10.0.0.2:8765', metadata)

    expect(store.getSnapshot()).toEqual({
      sources: [createSource('http://localhost:8765')],
      activeUrl: 'http://localhost:8765',
    })
    expect(store.add(remote)).toEqual({ source: remote, added: true })
    store.select(remote.baseUrl)

    expect(createSourceStore({ origin: 'http://localhost:8765', storage }).getSnapshot()).toEqual({
      sources: [createSource('http://localhost:8765'), remote],
      activeUrl: remote.baseUrl,
    })
  })

  it('deduplicates normalized URLs and refreshes cached instance metadata', () => {
    const store = createSourceStore({ origin: 'http://localhost:8765', storage: null })
    const notify = vi.fn<() => void>()
    store.subscribe(notify)

    const first = store.add(createSource('HTTP://REMOTE:80/', metadata))
    const duplicate = store.add(createSource('http://remote', metadata))

    expect(first.added).toBe(true)
    expect(duplicate).toEqual({ source: first.source, added: false })
    expect(store.getSnapshot().sources).toHaveLength(2)
    expect(notify).toHaveBeenCalledTimes(2)
  })

  it('refreshes the seeded local source after instance validation', () => {
    const store = createSourceStore({ origin: 'http://localhost:8765', storage: null })

    expect(store.add(createSource('http://localhost:8765', metadata)).added).toBe(false)
    expect(store.getSnapshot().sources).toEqual([createSource('http://localhost:8765', metadata)])
  })

  it('falls back to local only when the active remote is removed', () => {
    const store = createSourceStore({ origin: 'http://localhost:8765', storage: null })
    const first = createSource('http://10.0.0.2:8765', metadata)
    const second = createSource('http://10.0.0.3:8765', metadata)
    store.add(first)
    store.add(second)
    store.select(first.baseUrl)

    store.remove(second.baseUrl)
    expect(store.getSnapshot().activeUrl).toBe(first.baseUrl)
    store.remove(first.baseUrl)
    expect(store.getSnapshot().activeUrl).toBe('http://localhost:8765')
    expect(() => store.remove('http://localhost:8765')).toThrow('Local source cannot be removed')
  })

  it('keeps changes in memory when browser storage is unavailable', () => {
    const store = createSourceStore({
      origin: 'http://localhost:8765',
      storage: new ThrowingStorage(),
    })
    const remote = createSource('http://remote:8765', metadata)

    store.add(remote)
    store.select(remote.baseUrl)

    expect(store.getSnapshot().activeUrl).toBe(remote.baseUrl)
    expect(store.getSnapshot().sources).toHaveLength(2)
  })

  it('recovers corrupt or obsolete persisted state', () => {
    const storage = new MemoryStorage()
    storage.setItem(
      'tokeninsights.sources.v1',
      JSON.stringify({ version: 2, sources: [], activeUrl: 'http://remote:8765' }),
    )

    expect(createSourceStore({ origin: 'http://localhost:8765', storage }).getSnapshot()).toEqual({
      sources: [createSource('http://localhost:8765')],
      activeUrl: 'http://localhost:8765',
    })
  })
})
