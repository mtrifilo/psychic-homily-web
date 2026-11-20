import type { ComponentType, ReactElement } from 'react'
import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { renderToString } from 'react-dom/server'
import { QueryClient, QueryClientProvider, type QueryKey } from '@tanstack/react-query'
import { CachedListFallback } from '@/components/shared/CachedListFallback'
import {
  boundaryStateOf,
  entriesFor,
  renderOnNavigation,
  warmClient,
} from '@/test/firstScreenRender'

/** What one seeded route's fallback is checked against. */
export interface CachedListFallbackCase {
  /** The route, for test names. */
  route: string
  /** The `Suspense` fallback the route renders, on a bare URL. */
  fallback: () => Promise<ReactElement> | ReactElement
  /** The route's seeded boundary on a bare URL, with every read answering. */
  seededBoundary: () => Promise<ReactElement>
  /** The list the fallback must render over a warm cache. */
  list: ComponentType
  /**
   * Every key the route seeds, each with a payload, in the route's order. The
   * first is the rows; any others are facets every view of the list shares.
   */
  seeded: ReadonlyArray<readonly [QueryKey, unknown]>
  /** The params the route's seed gate skips on. */
  defeatingParams: readonly string[]
  /** Markup only the seeded payload's rows produce (an entity href). */
  seededRow: string
  /**
   * Markup the route's loader carries, and the list's own first-load state
   * when that differs. Any of them counts as a loader on screen.
   */
  loaders: readonly string[]
  /**
   * A cached entry for another filter, sort or page of the same list, with a
   * row only it produces, and the URL search string that reads it. Absent for
   * a list with no URL state.
   */
  otherQuery?: {
    entry: readonly [QueryKey, unknown]
    row: string
    search: string
  }
  /** The URL the file's mocked `useSearchParams` returns. Absent for a list with no URL state. */
  nav?: { params: URLSearchParams }
  /** The file's mocked `apiRequest`: every browser request it sees never answers. */
  apiRequest: Mock
}

/**
 * The behaviour every route's `CachedListFallback` has to show with its real
 * list: the loader in server HTML, the cached rows on a warm client-side
 * navigation, the loader on a cold one, and never another query's rows.
 */
export function describeCachedListFallback(c: CachedListFallbackCase) {
  const seededKeys = c.seeded.map(([key]) => key)
  const showsLoader = (html: string) => c.loaders.some(marker => html.includes(marker))

  describe(`${c.route} fallback on a client-side navigation`, () => {
    const setSearch = (search: string) => {
      if (c.nav) c.nav.params = new URLSearchParams(search)
    }

    beforeEach(() => {
      vi.clearAllMocks()
      c.apiRequest.mockReturnValue(new Promise(() => {}))
      setSearch('')
    })

    it('is the shared cached fallback over the real list and the seeded keys', async () => {
      const fallback = await c.fallback()
      const props = fallback.props as {
        seededKeys: readonly QueryKey[]
        defeatingParams?: readonly string[]
        children: ReactElement
      }

      expect(fallback.type).toBe(CachedListFallback)
      expect(props.seededKeys).toEqual(seededKeys)
      expect(props.defeatingParams ?? []).toEqual(c.defeatingParams)
      expect(props.children.type).toBe(c.list)
    })

    // The gate and the seed must name the same keys, or the fallback can run
    // the list over a seeded key the cache does not hold.
    it('gates on exactly the keys the seeded boundary seeds', async () => {
      const seededBoundary = await c.seededBoundary()

      expect(boundaryStateOf(seededBoundary).queries.map(q => q.queryKey)).toEqual(
        seededKeys
      )
    })

    it('server-renders the loader even when the cache is warm', async () => {
      const html = renderToString(
        <QueryClientProvider client={warmClient(c.seeded)}>
          {await c.fallback()}
        </QueryClientProvider>
      )

      expect(showsLoader(html)).toBe(true)
      expect(html).not.toContain(c.seededRow)
    })

    // The regression: the fallback was the loader whatever the cache held.
    it('draws the cached rows, with no loader, on a warm cache', async () => {
      const client = warmClient(c.seeded)
      const seededEntries = entriesFor(client, seededKeys)

      const { container } = renderOnNavigation(await c.fallback(), client)

      expect(container.innerHTML).toContain(c.seededRow)
      expect(showsLoader(container.innerHTML)).toBe(false)
      // The list's hooks ran, and every seeded entry is the one that was there.
      expect(entriesFor(client, seededKeys)).toEqual(seededEntries)
    })

    it('renders the loader and creates no seeded entry on a cold cache', async () => {
      const client = new QueryClient()

      const { container } = renderOnNavigation(await c.fallback(), client)

      expect(showsLoader(container.innerHTML)).toBe(true)
      expect(entriesFor(client, seededKeys).every(e => e === undefined)).toBe(true)
    })

    if (c.otherQuery) {
      const other = c.otherQuery
      const sharedFacets = c.seeded.slice(1)

      it('renders the loader, not the cached rows, when the cache holds only another query', async () => {
        const client = warmClient([other.entry, ...sharedFacets])

        const { container } = renderOnNavigation(await c.fallback(), client)

        expect(showsLoader(container.innerHTML)).toBe(true)
        expect(container.innerHTML).not.toContain(other.row)
        expect(entriesFor(client, [seededKeys[0]])).toEqual([undefined])
      })

      it('shows the list loading, not the first screen, on another query the cache lacks', async () => {
        setSearch(other.search)
        const client = warmClient(c.seeded)

        const { container } = renderOnNavigation(await c.fallback(), client)

        expect(showsLoader(container.innerHTML)).toBe(true)
        expect(container.innerHTML).not.toContain(c.seededRow)
      })

      it('draws the other query from the cache when the URL asks for it', async () => {
        setSearch(other.search)
        const client = warmClient([other.entry, ...sharedFacets])

        const { container } = renderOnNavigation(await c.fallback(), client)

        expect(container.innerHTML).toContain(other.row)
        expect(showsLoader(container.innerHTML)).toBe(false)
      })
    }
  })
}
