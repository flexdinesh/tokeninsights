import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  useSyncExternalStore,
} from 'react'
import type { ReactNode } from 'react'
import { createSourceStore } from './sources'
import type { Source, SourceStore } from './sources'

const SourceContext = createContext<SourceStore | null>(null)

export function SourceProvider({ children, store }: { children: ReactNode; store?: SourceStore }) {
  const [sourceStore] = useState(() => store ?? createSourceStore())
  return <SourceContext.Provider value={sourceStore}>{children}</SourceContext.Provider>
}

export function useSources() {
  const store = useContext(SourceContext)
  if (!store) throw new Error('SourceProvider missing')
  const subscribe = useCallback((listener: () => void) => store.subscribe(listener), [store])
  const getSnapshot = useCallback(() => store.getSnapshot(), [store])
  const add = useCallback((source: Source) => store.add(source), [store])
  const select = useCallback((baseUrl: string) => store.select(baseUrl), [store])
  const remove = useCallback((baseUrl: string) => store.remove(baseUrl), [store])
  const snapshot = useSyncExternalStore(subscribe, getSnapshot)
  const active = snapshot.sources.find((source) => source.baseUrl === snapshot.activeUrl)
  if (!active) throw new Error('Active source missing')
  return useMemo(
    () => ({
      ...snapshot,
      active,
      add,
      select,
      remove,
    }),
    [active, add, remove, select, snapshot],
  )
}
