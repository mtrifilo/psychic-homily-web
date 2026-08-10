import { beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchSeoList } = vi.hoisted(() => ({
  fetchSeoList: vi.fn(),
}))

vi.mock('@/lib/seo/fetchSeoList', () => ({ fetchSeoList }))

import { ARTIST_ITEM_LIST_LIMIT, getArtistsForMetadata } from './artistsMetadata'

const listingEntries = (count: number) =>
  Array.from({ length: count }, (_, i) => ({
    name: `Artist ${i}`,
    slug: `artist-${i}`,
  }))

// The fail-open behaviour itself belongs to fetchSeoList and is tested against
// the real implementation in lib/seo/fetchSeoList.test.ts. What is artists-
// specific — and what would silently regress the JSON-LD if it drifted — is the
// wiring: which endpoint, which collection key, which Sentry tag.
describe('getArtistsForMetadata', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // The endpoint is the assertion that matters (PSY-1674). Pointing this back
  // at `GET /artists` would restore a body 2x over the 2 MB cache-item cap, so
  // the entry stops being written: one console.warn buried in the build log,
  // nothing failing, an ItemList that still renders correctly, and every render
  // re-pulling the whole catalogue from origin. The build gate in
  // lib/data-cache-budget is the backstop; this test is the cheap first line.
  it('asks fetchSeoList for the artists listing projection, not the full list', async () => {
    const artists = [{ name: 'Desert Static', slug: 'desert-static' }]
    fetchSeoList.mockResolvedValue(artists)

    await expect(getArtistsForMetadata()).resolves.toEqual(artists)
    expect(fetchSeoList).toHaveBeenCalledWith({
      url: expect.stringMatching(/\/artists\/listing$/),
      collection: 'artists',
      service: 'artists-listing',
    })
  })

  // No timeoutMs: the 30s override was a bandaid for the oversized payload and
  // its reason is gone, so the call must fall through to the shared default
  // rather than carry a stale widened budget.
  it('does not override the shared fetch budget', async () => {
    fetchSeoList.mockResolvedValue([])

    await getArtistsForMetadata()
    expect(fetchSeoList.mock.calls[0][0]).not.toHaveProperty('timeoutMs')
  })
})

/**
 * The bound on the `ItemList` (PSY-1773).
 *
 * Unbounded, this block serialised 6,155 entries into a 723 KB `<script>` and
 * the same 723 KB into the RSC flight payload, which `<Link>` prefetch pulled
 * onto `/`, `/atlas` and `/shows`. Nothing failed then either — the page
 * rendered correctly and was simply enormous — so the bound is asserted here
 * rather than left to review.
 *
 * The second test is the one that is easy to lose. `GET /artists/listing` takes
 * NO query parameters (`ListArtistListingRequest` is an empty struct) and huma
 * ignores a parameter no input field declares, so rewriting the slice as
 * `?limit=100` would be accepted, ignored, and quietly restore all 6,155
 * entries while the call site still read as bounded. That is the silent-drift
 * failure `useVenuesFirstScreen.test.tsx` guards against on `/venues`, in the
 * shape this route can actually exhibit: there is no first-screen seed to
 * assert here, because `ArtistList` still fetches the unbounded `GET /artists`
 * client-side and cannot be seeded until PSY-1774 bounds it.
 */
describe('ItemList bound', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('caps the entries it returns even when the endpoint sends the catalogue', async () => {
    fetchSeoList.mockResolvedValue(listingEntries(6_155))

    await expect(getArtistsForMetadata()).resolves.toHaveLength(
      ARTIST_ITEM_LIST_LIMIT
    )
  })

  it('bounds by slicing, never by a query parameter the endpoint ignores', async () => {
    fetchSeoList.mockResolvedValue(listingEntries(6_155))

    await getArtistsForMetadata()
    expect(fetchSeoList.mock.calls[0][0].url).not.toMatch(/limit/)
  })

  // Order is the selection. `/artists/listing` shares `artistBrowseOrder` with
  // the browse page (upcoming show count DESC, then name ASC), so taking the
  // head keeps the 100 MOST ACTIVE artists. A slice from anywhere else, or a
  // sort applied on the way through, would silently change which artists get
  // the enrichment.
  it('keeps the head of the endpoint order, the most active artists', async () => {
    fetchSeoList.mockResolvedValue(listingEntries(6_155))

    const artists = await getArtistsForMetadata()
    expect(artists[0]).toEqual({ name: 'Artist 0', slug: 'artist-0' })
    expect(artists.at(-1)).toEqual({ name: 'Artist 99', slug: 'artist-99' })
  })

  it('passes a short list through untouched', async () => {
    const artists = listingEntries(3)
    fetchSeoList.mockResolvedValue(artists)

    await expect(getArtistsForMetadata()).resolves.toEqual(artists)
  })

  it('matches the bound /venues applies to its own ItemList', () => {
    expect(ARTIST_ITEM_LIST_LIMIT).toBe(100)
  })
})
