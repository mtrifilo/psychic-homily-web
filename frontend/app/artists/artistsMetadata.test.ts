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
 * Unbounded, the page emitted one entry per artist into the `<script>` AND again
 * into the RSC flight payload — 1.65 MiB of HTML, 124 KB on the wire, and a
 * `<Link>` prefetch that carried it onto `/`, `/atlas` and `/shows`. Nothing
 * failed: the page rendered correctly and was simply enormous. That is why the
 * bound is asserted rather than left to review.
 *
 * These tests pin `getArtistsForMetadata`. The ARTIFACT is the `<script>` that
 * `page.tsx` renders from it, and `page.test.tsx` pins that end separately —
 * without it, inlining the fetch back into the page would restore the bug with
 * every test here still green.
 */
describe('ItemList bound', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // The mechanism-independent guard: whatever bounds the list, the list is
  // bounded. This one survives PSY-1794 moving the bound into the URL.
  it('caps the entries it returns even when the endpoint sends the catalogue', async () => {
    fetchSeoList.mockResolvedValue(listingEntries(6_200))

    await expect(getArtistsForMetadata()).resolves.toHaveLength(
      ARTIST_ITEM_LIST_LIMIT
    )
  })

  /**
   * TODAY'S MECHANISM, and the trap it guards.
   *
   * `GET /artists/listing` declares no query parameters at all
   * (`ListArtistListingRequest` is an empty struct) and huma ignores one no
   * input field declares. So rewriting the slice as `?limit=100` would be
   * accepted, ignored, and quietly restore the whole catalogue while the call
   * site still READ as bounded.
   *
   * DELETE THIS TEST, deliberately, when the endpoint grows a real `limit` —
   * at that point sending one is the correct implementation and this assertion
   * becomes wrong. The test above is what must keep passing across that change.
   */
  it('does not send a limit the endpoint would silently ignore', async () => {
    fetchSeoList.mockResolvedValue(listingEntries(6_200))

    await getArtistsForMetadata()
    expect(fetchSeoList.mock.calls[0][0].url).not.toMatch(/limit/)
  })

  // WHICH end of the list is kept. The endpoint sorts most-active first
  // (`artistBrowseOrder`), so the head is the selection worth advertising; this
  // pins that we take the head and never re-sort or sample. The ordering itself
  // is the backend's guarantee, not something this test can observe.
  it('keeps the head of whatever order the endpoint returned', async () => {
    fetchSeoList.mockResolvedValue(listingEntries(6_200))

    const artists = await getArtistsForMetadata()
    expect(artists[0]).toEqual({ name: 'Artist 0', slug: 'artist-0' })
    expect(artists.at(-1)).toEqual({ name: 'Artist 99', slug: 'artist-99' })
  })

  it('passes a short list through untouched', async () => {
    const artists = listingEntries(3)
    fetchSeoList.mockResolvedValue(artists)

    await expect(getArtistsForMetadata()).resolves.toEqual(artists)
  })
})
