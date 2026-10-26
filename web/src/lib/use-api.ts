import { useCallback, useEffect, useRef, useState } from "react"
import { api } from "./api"

// A tiny stale-while-revalidate cache: revisiting a page shows the last
// response immediately and refreshes it in the background.
const cache = new Map<string, unknown>()

export function useApi<T>(path: string | null) {
  const [data, setData] = useState<T | undefined>(() => (path ? (cache.get(path) as T | undefined) : undefined))
  const [error, setError] = useState<Error | null>(null)
  const [loading, setLoading] = useState(false)
  const current = useRef(path)

  const reload = useCallback(async () => {
    if (!path) return
    setLoading(true)
    try {
      const d = await api<T>(path)
      cache.set(path, d)
      if (current.current === path) {
        setData(d)
        setError(null)
      }
    } catch (e) {
      if (current.current === path) setError(e as Error)
    } finally {
      if (current.current === path) setLoading(false)
    }
  }, [path])

  useEffect(() => {
    current.current = path
    setData(path ? (cache.get(path) as T | undefined) : undefined)
    reload()
  }, [path, reload])

  return { data, error, loading, reload, setData }
}

export function invalidate(prefix: string) {
  for (const k of cache.keys()) if (k.startsWith(prefix)) cache.delete(k)
}
