import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReactElement } from 'react'

const { fetchSeoList, fetchListPayload, seedFirstScreen } = vi.hoisted(() => ({
  fetchSeoList: vi.fn(),
  fetchListPayload: vi.fn(),
  seedFirstScreen: vi.fn(),
}))
vi.mock('@/lib/seo/fetchSeoList', () => ({ fetchSeoList }))
vi.mock('@/lib/ssr/fetchListPayload', () => ({ fetchListPayload }))
vi.mock('@/lib/query-hydration', () => ({ seedFirstScreen }))

// `ArtistList` is a client component pulling in nuqs, TanStack Query and the
// whole filter surface. This suite is about the JSON-LD the SERVER emits, so the
// browse list is stubbed to a marker: rendering it would test other people's
// modules and drag a router context into a test that has no DOM.
vi.mock('@/features/artists', () => ({
  ArtistList: () => null,
  ArtistListSkeleton: () => null,
}))

import ArtistsPage from './page'
import { ARTIST_ITEM_LIST_LIMIT } from './artistsMetadata'
import {
  ARTIST_LIST_FIRST_SCREEN_KEY,
  ARTIST_LIST_FIRST_SCREEN_URL,
  artistEndpoints,
  artistQueryKeys,
} from '@/features/artists/api'

const listingEntries = (count: number) =>
  Array.from({ length: count }, (_, i) => ({
    name: `Artist ${i}`,
    slug: `artist-${i}`,
  }))

/** Every JSON-LD payload the page rendered, in order. */
function jsonLdPayloads(tree: ReactElement): Array<Record<string, unknown>> {
  const found: Array<Record<string, unknown>> = []
  const walk = (node: unknown): void => {
    if (Array.isArray(node)) {
      node.forEach(walk)
      return
    }
    if (!node || typeof node !== 'object') return
    const el = node as { props?: Record<string, unknown> }
    const props = el.props
    if (props && typeof props === 'object') {
      if ('data' in props && props.data && typeof props.data === 'object') {
        found.push(props.data as Record<string, unknown>)
      }
      if ('children' in props) walk(props.children)
    }
  }
  walk(tree)
  return found
}

const itemList = (tree: ReactElement) =>
  jsonLdPayloads(tree).find(d => d['@type'] === 'ItemList')

/** A bare `/artists` — no search params, the URL the first-screen seed covers. */
const noSearchParams = () => ({ searchParams: Promise.resolve({}) })

/** `/artists?<key>=<value>` — a URL that keys off the first-screen entry. */
const withSearchParam = (key: string, value: string) => ({
  searchParams: Promise.resolve({ [key]: value }),
})

/**
 * The `ItemList` bound, asserted on the ARTIFACT (PSY-1773).
 *
 * `artistsMetadata.test.ts` pins `getArtistsForMetadata`. This pins the block
 * the page actually emits, which is the thing that was 794 KB. The two are
 * separate on purpose: inlining the fetch back into this file — or rendering
 * some other list into the schema — would restore the bug with every test in
 * the sibling file still green.
 */
describe('/artists JSON-LD ItemList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('emits at most ARTIST_ITEM_LIST_LIMIT entries for a full catalogue', async () => {
    fetchSeoList.mockResolvedValue(listingEntries(6_200))

    const list = itemList(await ArtistsPage(noSearchParams()))

    expect(list).toBeDefined()
    expect(list!.itemListElement).toHaveLength(ARTIST_ITEM_LIST_LIMIT)
  })

  // schema.org consistency has to hold AT the bound, not just unbounded:
  // `numberOfItems` is derived from the array, and `position` is 1-based and
  // contiguous, so a truncated block stays internally honest.
  it('keeps numberOfItems and positions consistent at the bound', async () => {
    fetchSeoList.mockResolvedValue(listingEntries(6_200))

    const list = itemList(await ArtistsPage(noSearchParams()))
    const elements = list!.itemListElement as Array<{ position: number }>

    expect(list!.numberOfItems).toBe(ARTIST_ITEM_LIST_LIMIT)
    expect(elements.map(e => e.position)).toEqual(
      Array.from({ length: ARTIST_ITEM_LIST_LIMIT }, (_, i) => i + 1)
    )
  })

  it('advertises the head of the endpoint order', async () => {
    fetchSeoList.mockResolvedValue(listingEntries(6_200))

    const elements = itemList(await ArtistsPage(noSearchParams()))!.itemListElement as Array<{
      url: string
    }>

    expect(elements[0].url).toBe('https://psychichomily.com/artists/artist-0')
    expect(elements.at(-1)!.url).toBe('https://psychichomily.com/artists/artist-99')
  })

  // The fail-open path: `fetchSeoList` returns [] on a backend blip, and the
  // page must then emit no ItemList at all rather than an empty one, which
  // would advertise "this site has zero artists" to a crawler.
  it('emits no ItemList when the fetch fails open', async () => {
    fetchSeoList.mockResolvedValue([])

    expect(itemList(await ArtistsPage(noSearchParams()))).toBeUndefined()
  })

  // The breadcrumb is unconditional and must survive a failed listing fetch.
  it('always emits the breadcrumb', async () => {
    fetchSeoList.mockResolvedValue([])

    const crumb = jsonLdPayloads(await ArtistsPage(noSearchParams())).find(
      d => d['@type'] === 'BreadcrumbList'
    )
    expect(crumb).toBeDefined()
  })
})

/**
 * The SERVER half of the first-screen pair (PSY-1774).
 *
 * `useArtistsFirstScreen.test.tsx` pins what the hooks request and key on. This
 * pins what the page fetches and seeds. Both halves have to name the same
 * constants, or the seed silently misses and the page reverts to rendering a
 * spinner on the server: a failure that looks like nothing at all.
 */
describe('/artists server-seeded first screen', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchSeoList.mockResolvedValue([])
    seedFirstScreen.mockResolvedValue({ mutations: [], queries: [] })
  })

  /**
   * Invokes the async component nested under the page's Suspense boundary.
   * Reached through the rendered tree rather than by exporting it, so the
   * seeding stays an implementation detail of the route, which is what it is.
   */
  async function renderHydratedList(
    pageProps: { searchParams: Promise<Record<string, string | string[] | undefined>> } =
      noSearchParams(),
  ) {
    const walk = async (node: unknown): Promise<ReactElement | undefined> => {
      if (Array.isArray(node)) {
        for (const child of node) {
          const hit = await walk(child)
          if (hit) return hit
        }
        return undefined
      }
      if (!node || typeof node !== 'object') return undefined
      const el = node as { type?: unknown; props?: Record<string, unknown> }
      if (
        typeof el.type === 'function' &&
        (el.type as { name?: string }).name === 'HydratedArtistList'
      ) {
        return (await (el.type as (p: unknown) => Promise<ReactElement>)(
          el.props
        )) as ReactElement
      }
      if (el.props && 'children' in el.props) return walk(el.props.children)
      return undefined
    }
    const found = await walk(await ArtistsPage(pageProps))
    expect(found, 'HydratedArtistList was not found under the page').toBeDefined()
    return found!
  }

  it('fetches the paired first-screen URL and the city facets', async () => {
    fetchListPayload.mockResolvedValue({ artists: [], total: 0, limit: 50, offset: 0 })

    await renderHydratedList()

    expect(fetchListPayload).toHaveBeenCalledWith(
      expect.objectContaining({
        url: ARTIST_LIST_FIRST_SCREEN_URL,
        collection: 'artists',
      })
    )
    // BOTH are required: ArtistList renders its spinner while EITHER query is
    // loading, so seeding the rows alone server-renders a spinner.
    expect(fetchListPayload).toHaveBeenCalledWith(
      expect.objectContaining({
        url: artistEndpoints.CITIES,
        collection: 'cities',
      })
    )
  })

  it('seeds the exact keys the hooks register', async () => {
    fetchListPayload.mockResolvedValue({ artists: [], total: 0, limit: 50, offset: 0 })

    await renderHydratedList()

    expect(seedFirstScreen).toHaveBeenCalledWith([
      { queryKey: ARTIST_LIST_FIRST_SCREEN_KEY, data: expect.anything() },
      { queryKey: artistQueryKeys.cities, data: expect.anything() },
    ])
  })

  it('renders the list unseeded when a fetch fails, rather than throwing', async () => {
    fetchListPayload.mockResolvedValue(null)

    await renderHydratedList()

    // The component fetches for itself and owns the error state; handing
    // error.tsx a page the browser could have rendered would be worse.
    expect(seedFirstScreen).not.toHaveBeenCalled()
  })

  // The seed describes ONE request. On a URL the hook keys away from, fetching
  // it anyway costs two Data Cache reads and ships a dehydrated payload nothing
  // ever reads. `?page=` made that the common deep link rather than a rare one.
  it.each([
    ['page', '2'],
    ['cities', 'Phoenix,AZ'],
    ['tags', 'post-punk'],
    ['tag_match', 'any'],
  ])('skips the seed entirely when ?%s= is present', async (key, value) => {
    fetchListPayload.mockResolvedValue({ artists: [], total: 0, limit: 50, offset: 0 })

    await renderHydratedList(withSearchParam(key, value))

    expect(fetchListPayload).not.toHaveBeenCalled()
    expect(seedFirstScreen).not.toHaveBeenCalled()
  })
})
