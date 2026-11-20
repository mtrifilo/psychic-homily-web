import { Suspense, type ReactElement } from 'react'
import { renderToString } from 'react-dom/server'
import { render } from '@testing-library/react'
import { CachedListFallback } from '@/components/shared/CachedListFallback'
import { findElement } from '@/test/elementTree'
import { renderWithProviders } from '@/test/utils'
import {
  QueryClient,
  QueryClientProvider,
  type DehydratedState,
  type QueryKey,
} from '@tanstack/react-query'

/** The class every list loader on these routes spins with. */
export const LOADER = 'animate-spin'

const testClient = () =>
  new QueryClient({ defaultOptions: { queries: { retry: false } } })

/**
 * A seeded boundary rendered the way the route's HTML is produced: no client
 * fetch runs inside `renderToString`, so whatever this prints came from the
 * seed.
 */
export function serverRenderFirstScreen(boundary: ReactElement): string {
  return renderToString(
    <QueryClientProvider client={testClient()}>{boundary}</QueryClientProvider>
  )
}

/** The dehydrated state a `HydrationBoundary` element carries. */
export const boundaryStateOf = (boundary: ReactElement) =>
  (boundary.props as { state: DehydratedState }).state

/**
 * Mount a list in an empty cache and report what it asked for: the hash of
 * every query it registered. The caller's `apiRequest` mock records the URLs.
 */
export function registeredQueryHashes(list: ReactElement): string[] {
  const client = testClient()
  render(<QueryClientProvider client={client}>{list}</QueryClientProvider>)
  return client
    .getQueryCache()
    .getAll()
    .map(query => query.queryHash)
}

/**
 * `URLSearchParams` that remembers every name a component reads, so a test can
 * compare the params a list depends on with the params its seed gate skips on.
 */
export class RecordingSearchParams extends URLSearchParams {
  readonly read = new Set<string>()

  get(name: string): string | null {
    this.read.add(name)
    return super.get(name)
  }

  getAll(name: string): string[] {
    this.read.add(name)
    return super.getAll(name)
  }

  has(name: string, value?: string): boolean {
    this.read.add(name)
    return value === undefined ? super.has(name) : super.has(name, value)
  }
}

/**
 * The `CachedListFallback` a route's returned tree hands a `Suspense`, found
 * by type, so another `Suspense` earlier in the tree is never taken for it.
 */
export const cachedListFallbackOf = (tree: ReactElement): ReactElement =>
  (findElement(
    tree,
    node =>
      node.type === Suspense &&
      (node.props as { fallback?: ReactElement }).fallback?.type ===
        CachedListFallback
  ).props as { fallback: ReactElement }).fallback

/**
 * A fallback rendered the way a client-side navigation renders it: a mount on
 * the browser's own client, not a hydration, so the cache it holds is read.
 */
export const renderOnNavigation = (element: ReactElement, client: QueryClient) =>
  renderWithProviders(element, { queryClient: client })

/** A browser client with `entries` cached, the way an earlier visit leaves it. */
export function warmClient(
  entries: ReadonlyArray<readonly [QueryKey, unknown]>
): QueryClient {
  const client = testClient()
  for (const [queryKey, data] of entries) client.setQueryData(queryKey, data)
  return client
}

/**
 * The cache entry object behind each key, or `undefined` for a key with none.
 * Comparing two snapshots shows whether a render created or replaced a
 * seeded key's entry.
 */
export function entriesFor(client: QueryClient, keys: readonly QueryKey[]) {
  return keys.map(queryKey =>
    client.getQueryCache().find({ queryKey, exact: true })
  )
}
