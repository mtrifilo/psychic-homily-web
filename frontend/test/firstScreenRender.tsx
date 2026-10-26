import type { ReactElement } from 'react'
import { renderToString } from 'react-dom/server'
import { render } from '@testing-library/react'
import {
  QueryClient,
  QueryClientProvider,
  type DehydratedState,
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
