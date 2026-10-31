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
