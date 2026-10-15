import { QueryClient } from '@tanstack/react-query'
import { defaultQueryOptions } from '@/lib/queryClient'
import type { Comment } from '../types'

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

/** A promise plus the function that resolves it, for holding a response. */
export function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => (resolve = r))
  return { promise, resolve }
}

export function makeComment(overrides: Partial<Comment> = {}): Comment {
  return {
    id: 1,
    entity_type: 'venue',
    entity_id: 42,
    user_id: 7,
    author_name: 'Author',
    body: 'body',
    body_html: '<p>body</p>',
    parent_id: null,
    root_id: null,
    depth: 0,
    ups: 0,
    downs: 0,
    score: 0,
    visibility: 'visible',
    reply_permission: 'anyone',
    edit_count: 0,
    is_edited: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
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
