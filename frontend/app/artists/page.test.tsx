import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReactElement } from 'react'

const { fetchSeoList } = vi.hoisted(() => ({ fetchSeoList: vi.fn() }))
vi.mock('@/lib/seo/fetchSeoList', () => ({ fetchSeoList }))

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

    const list = itemList(await ArtistsPage())

    expect(list).toBeDefined()
    expect(list!.itemListElement).toHaveLength(ARTIST_ITEM_LIST_LIMIT)
  })

  // schema.org consistency has to hold AT the bound, not just unbounded:
  // `numberOfItems` is derived from the array, and `position` is 1-based and
  // contiguous, so a truncated block stays internally honest.
  it('keeps numberOfItems and positions consistent at the bound', async () => {
    fetchSeoList.mockResolvedValue(listingEntries(6_200))

    const list = itemList(await ArtistsPage())
    const elements = list!.itemListElement as Array<{ position: number }>

    expect(list!.numberOfItems).toBe(ARTIST_ITEM_LIST_LIMIT)
    expect(elements.map(e => e.position)).toEqual(
      Array.from({ length: ARTIST_ITEM_LIST_LIMIT }, (_, i) => i + 1)
    )
  })

  it('advertises the head of the endpoint order', async () => {
    fetchSeoList.mockResolvedValue(listingEntries(6_200))

    const elements = itemList(await ArtistsPage())!.itemListElement as Array<{
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

    expect(itemList(await ArtistsPage())).toBeUndefined()
  })

  // The breadcrumb is unconditional and must survive a failed listing fetch.
  it('always emits the breadcrumb', async () => {
    fetchSeoList.mockResolvedValue([])

    const crumb = jsonLdPayloads(await ArtistsPage()).find(
      d => d['@type'] === 'BreadcrumbList'
    )
    expect(crumb).toBeDefined()
  })
})
