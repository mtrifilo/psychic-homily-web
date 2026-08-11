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

import { HydrationBoundary } from '@tanstack/react-query'
import ArtistsPage, {
  FirstScreenItemList,
  HydratedArtistList,
} from './page'
import { ARTIST_ITEM_LIST_LIMIT, getArtistsForMetadata } from './artistsMetadata'
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

/**
 * The `ItemList` payload for a URL, if the page emits one there.
 *
 * The block lives in its own async component under Suspense — reading
 * `searchParams` in the page body would make the whole route dynamic — so it is
 * resolved directly rather than found by walking the synchronous tree.
 */
const itemList = async (
  params: Record<string, string | string[] | undefined> = {}
) => {
  const artists = await getArtistsForMetadata()
  const withSlugs = artists.filter(
    (a): a is (typeof artists)[number] & { slug: string } => !!a.slug
  )
  if (withSlugs.length === 0) return undefined
  const block = await FirstScreenItemList({
    searchParams: Promise.resolve(params),
    artists: withSlugs,
  })
  return block ? (block.props.data as Record<string, unknown>) : undefined
}

/** A bare `/artists` — no search params, the URL the first-screen seed covers. */
const noSearchParams = () => ({ searchParams: Promise.resolve({}) })

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

    const list = await itemList()

    expect(list).toBeDefined()
    expect(list!.itemListElement).toHaveLength(ARTIST_ITEM_LIST_LIMIT)
  })

  // schema.org consistency has to hold AT the bound, not just unbounded:
  // `numberOfItems` is derived from the array, and `position` is 1-based and
  // contiguous, so a truncated block stays internally honest.
  it('keeps numberOfItems and positions consistent at the bound', async () => {
    fetchSeoList.mockResolvedValue(listingEntries(6_200))

    const list = await itemList()
    const elements = list!.itemListElement as Array<{ position: number }>

    expect(list!.numberOfItems).toBe(ARTIST_ITEM_LIST_LIMIT)
    expect(elements.map(e => e.position)).toEqual(
      Array.from({ length: ARTIST_ITEM_LIST_LIMIT }, (_, i) => i + 1)
    )
  })

  it('advertises the head of the endpoint order', async () => {
    fetchSeoList.mockResolvedValue(listingEntries(6_200))

    const elements = (await itemList())!.itemListElement as Array<{
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

    expect(await itemList()).toBeUndefined()
  })

  // The block describes the 100 most active artists regardless of the page it
  // is on, so on `?page=7` it advertised artists 1-100 over a document showing
  // 301-350. PSY-1774 minted ~124 such URLs; it is emitted only where it is
  // true.
  it('emits no ItemList on a paginated or filtered URL', async () => {
    fetchSeoList.mockResolvedValue(listingEntries(6_200))

    expect(await itemList({ page: '7' })).toBeUndefined()
    expect(await itemList({ cities: 'Phoenix,AZ' })).toBeUndefined()
    expect(await itemList({ tags: 'post-punk' })).toBeUndefined()
    // ...and still emitted on the URL it does describe.
    expect(await itemList()).toBeDefined()
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
   * Renders the seeding component directly.
   *
   * It is imported rather than located by walking the page tree for a function
   * NAME: a name match breaks silently on a rename or a `memo` wrapper and
   * points the failure at the traversal helper instead of at the edit that
   * caused it. Next ignores extra named exports from a route module — the
   * sibling `app/venues/page.tsx` exports two of its own.
   */
  const renderHydratedList = (
    params: Record<string, string | string[] | undefined> = {}
  ) => HydratedArtistList({ searchParams: Promise.resolve(params) })

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

    const tree = await renderHydratedList()

    // The component fetches for itself and owns the error state; handing
    // error.tsx a page the browser could have rendered would be worse.
    expect(seedFirstScreen).not.toHaveBeenCalled()
    expect(tree.props.state).toEqual({ mutations: [], queries: [] })
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

    await renderHydratedList({ [key]: value })

    expect(fetchListPayload).not.toHaveBeenCalled()
    expect(seedFirstScreen).not.toHaveBeenCalled()
  })

  // The element type must NOT vary with the seed. A bare <ArtistList /> on the
  // skip paths remounts the list on the first page click off page 1 — the rows
  // are replaced by the full-page spinner and focus is dropped to <body>.
  it.each([
    ['seeded', {}],
    ['skipped by a filter', { page: '2' }],
    ['skipped by a failed fetch', {}],
  ])('renders the same boundary type when the seed is %s', async (label, params) => {
    fetchListPayload.mockResolvedValue(
      label === 'skipped by a failed fetch'
        ? null
        : { artists: [], total: 0, limit: 50, offset: 0 }
    )

    const tree = await renderHydratedList(params)

    expect(tree.type).toBe(HydrationBoundary)
  })

  // An empty value is not a filter. `?tags=` parses to no tags and `?page=`
  // parses to page 1, so both land on the first-screen key — reading them as
  // "present" would skip a seed that would have hit.
  it.each([['tags'], ['page'], ['cities']])(
    'still seeds when ?%s= carries an empty value',
    async key => {
      fetchListPayload.mockResolvedValue({ artists: [], total: 0, limit: 50, offset: 0 })

      await renderHydratedList({ [key]: '' })

      expect(seedFirstScreen).toHaveBeenCalled()
    }
  )
})
