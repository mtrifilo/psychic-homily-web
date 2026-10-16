import { QueryClient } from '@tanstack/react-query'
import { defaultQueryOptions } from '@/lib/queryClient'

/**
 * Fixtures for the in-flight ordering case: a query's first fetch is still in
 * flight when a write succeeds, and that fetch's response was produced before
 * the write committed. A test holds the first response with `deferred`, runs
 * the mutation, releases the stale response, and asserts a second request was
 * made and its data is what the query shows.
 */

/**
 * A client with the app's default query options, which decide whether anything
 * refetches on its own (staleTime, refetch on focus). It has none of the app
 * client's cache error handlers. Retries are off, so an unexpected request
 * settles as an error state rather than waiting out a retry delay.
 */
export function clientWithAppDefaults(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { ...defaultQueryOptions.queries, retry: false },
      mutations: { ...defaultQueryOptions.mutations, retry: false },
    },
  })
}

/** A promise plus the functions that settle it, for holding a response. */
export function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

/**
 * Routes `apiRequest` GETs by URL fragment. The first GET for a fragment gets
 * that route's `stale` promise (a response produced before the write); every
 * later GET gets `fresh`. `handle` returns undefined for anything it does not
 * route, so the caller can answer writes.
 */
export function routeGets(
  routes: Record<string, { stale: Promise<unknown>; fresh: unknown }>
) {
  const counts: Record<string, number> = {}
  const handle = (url: string, init?: RequestInit) => {
    if ((init?.method ?? 'GET') !== 'GET') return undefined
    const fragment = Object.keys(routes).find((f) => url.includes(f))
    if (!fragment) return undefined
    counts[fragment] = (counts[fragment] ?? 0) + 1
    return counts[fragment] === 1
      ? routes[fragment].stale
      : Promise.resolve(routes[fragment].fresh)
  }
  return { counts, handle }
}
