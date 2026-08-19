import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { hashKey } from '@tanstack/react-query'
import { createTestQueryClient, createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

// Deliberately NOT mocking '@/features/artists/api': the whole point of these
// tests is the REAL query key. The sibling useArtists.test.tsx mocks that
// module, so it cannot see this class of bug at all.
import {
  ARTIST_SHOWS_PAGE_LIMIT,
  artistPastShowsPageParams,
  artistQueryKeys,
} from '@/features/artists/api'
import { useArtistShows } from './useArtists'

const ARTIST_ID = 42

/** The compact page a panel preview asks for. */
const PREVIEW_LIMIT = 20

function fiftyShows() {
  return {
    shows: Array.from({ length: 50 }, (_, i) => ({ id: i + 1 })),
    total: 50,
  }
}

function twentyShows() {
  return {
    shows: Array.from({ length: 20 }, (_, i) => ({ id: i + 1 })),
    total: 50,
  }
}

/**
 * PSY-1754. `artistQueryKeys.shows()` plus the time filter used to be the whole
 * cache key for an artist-shows request, while `limit` varied per caller.
 * Every artist-shows surface therefore shared one entry regardless
 * of what it had actually asked for, and whichever request resolved first
 * answered for all of them for the full 5-minute staleTime.
 *
 * This is the artist twin of the venue bug PSY-1698 fixed; the artist side
 * never got the fix, and the past archive added two more ways to get it wrong
 * (`year` and `offset`) on top. The archive also both ISSUES a page request and
 * PEEKS at the cache for its neighbours, through two different code paths that
 * must agree on the key exactly — the last case here pins them together.
 */
describe('artist-shows cache key isolates differently-parameterized callers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('does not let a 20-row preview answer the artist page for the same artist', async () => {
    const queryClient = createTestQueryClient()
    const wrapper = createWrapperWithClient(queryClient)

    // The preview lands FIRST — the navigation order that used to poison the
    // artist page's entry.
    mockApiRequest.mockResolvedValueOnce(twentyShows())
    const preview = renderHook(
      () =>
        useArtistShows({
          artistId: ARTIST_ID,
          limit: PREVIEW_LIMIT,
        }),
      { wrapper },
    )
    await waitFor(() => expect(preview.result.current.isSuccess).toBe(true))

    // Then the artist page mounts against the same warm cache.
    mockApiRequest.mockResolvedValueOnce(fiftyShows())
    const artistPage = renderHook(
      () =>
        useArtistShows({
          artistId: ARTIST_ID,
          limit: ARTIST_SHOWS_PAGE_LIMIT,
          timeFilter: 'upcoming',
        }),
      { wrapper },
    )
    await waitFor(() => expect(artistPage.result.current.isSuccess).toBe(true))

    // The regression: before the fix this was 20, served from the preview's
    // entry without ever issuing the artist page's own request.
    expect(artistPage.result.current.data?.shows).toHaveLength(
      ARTIST_SHOWS_PAGE_LIMIT,
    )
    expect(preview.result.current.data?.shows).toHaveLength(PREVIEW_LIMIT)

    // Two questions, two requests, two entries.
    expect(mockApiRequest).toHaveBeenCalledTimes(2)
    expect(queryClient.getQueryCache().getAll()).toHaveLength(2)
  })

  it('keys the request on limit and time filter', async () => {
    const queryClient = createTestQueryClient()
    mockApiRequest.mockResolvedValue(fiftyShows())

    const { result } = renderHook(
      () =>
        useArtistShows({
          artistId: ARTIST_ID,
          limit: ARTIST_SHOWS_PAGE_LIMIT,
          timeFilter: 'upcoming',
        }),
      { wrapper: createWrapperWithClient(queryClient) },
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    expect(cached[0].queryHash).toBe(
      hashKey(
        artistQueryKeys.showsPage(ARTIST_ID, {
          timeFilter: 'upcoming',
          limit: ARTIST_SHOWS_PAGE_LIMIT,
        }),
      ),
    )
  })

  it('separates two pages of the same year, and two years of the same page', async () => {
    const queryClient = createTestQueryClient()
    mockApiRequest.mockResolvedValue(fiftyShows())
    const wrapper = createWrapperWithClient(queryClient)

    const requests = [
      { year: 2025, offset: 0 },
      { year: 2025, offset: 50 },
      { year: 2024, offset: 0 },
    ]
    for (const request of requests) {
      const { result } = renderHook(
        () =>
          useArtistShows({
            artistId: ARTIST_ID,
            timeFilter: 'past',
            limit: ARTIST_SHOWS_PAGE_LIMIT,
            ...request,
          }),
        { wrapper },
      )
      await waitFor(() => expect(result.current.isSuccess).toBe(true))
    }

    expect(queryClient.getQueryCache().getAll()).toHaveLength(requests.length)
  })

  it('still nests every page under the shows() invalidation prefix', async () => {
    const queryClient = createTestQueryClient()
    mockApiRequest.mockResolvedValue(fiftyShows())

    const wrapper = createWrapperWithClient(queryClient)
    const upcoming = renderHook(
      () =>
        useArtistShows({
          artistId: ARTIST_ID,
          limit: ARTIST_SHOWS_PAGE_LIMIT,
          timeFilter: 'upcoming',
        }),
      { wrapper },
    )
    const past = renderHook(
      () =>
        useArtistShows({
          artistId: ARTIST_ID,
          limit: PREVIEW_LIMIT,
          timeFilter: 'past',
        }),
      { wrapper },
    )
    await waitFor(() => expect(upcoming.result.current.isSuccess).toBe(true))
    await waitFor(() => expect(past.result.current.isSuccess).toBe(true))

    // Widening the key must not narrow invalidation: createInvalidateQueries
    // fans out from ['artists'], and mutations reach one artist's pages via
    // shows(). Both survive only while the extra segments are appended.
    const matched = queryClient
      .getQueryCache()
      .findAll({ queryKey: artistQueryKeys.shows(ARTIST_ID) })
    expect(matched).toHaveLength(2)
    expect(
      queryClient.getQueryCache().findAll({ queryKey: ['artists'] }),
    ).toHaveLength(2)
  })

  it('keys on what the URL actually sent, not on the raw argument', async () => {
    const queryClient = createTestQueryClient()
    mockApiRequest.mockResolvedValue(twentyShows())
    const wrapper = createWrapperWithClient(queryClient)

    // A falsy limit drops out of the URL, so the key has to record the absence
    // rather than the argument — the mirror image of the collision above, and
    // the reason the hook resolves the sent values once for both the URL and the
    // key.
    const zeroLimit = renderHook(
      () => useArtistShows({ artistId: ARTIST_ID, limit: 0 }),
      { wrapper },
    )
    const omitted = renderHook(
      () => useArtistShows({ artistId: ARTIST_ID, limit: undefined }),
      { wrapper },
    )
    await waitFor(() => expect(zeroLimit.result.current.isSuccess).toBe(true))
    await waitFor(() => expect(omitted.result.current.isSuccess).toBe(true))

    // `limit: undefined` takes the hook's own default of 20 and sends it;
    // `limit: 0` sends nothing. Different requests, so still two entries —
    // but each entry matches the URL that filled it.
    const urls = mockApiRequest.mock.calls.map(c => c[0] as string)
    expect(urls.some(u => !u.includes('limit='))).toBe(true)
    expect(urls.some(u => u.includes('limit=20'))).toBe(true)

    // And no request carries a timezone. The endpoint partitions on each show's
    // own venue-local calendar day and ignores a caller zone, so sending one
    // buys nothing and costs a cache entry per viewer. The option is gone from
    // the hook, so only the hook itself could put one back; that is exactly what
    // this catches.
    expect(urls.every(u => !u.includes('timezone='))).toBe(true)

    // And the KEYS record the same thing. Asserting only the URLs would pass
    // against the pre-PSY-1754 key too, which is the whole bug this file exists
    // for: both hooks must land on a key built from the SENT values.
    const hashes = queryClient
      .getQueryCache()
      .getAll()
      .map(entry => entry.queryHash)
    expect(hashes).toHaveLength(2)
    expect(new Set(hashes)).toEqual(
      new Set([
        hashKey(
          artistQueryKeys.showsPage(ARTIST_ID, { timeFilter: 'upcoming' }),
        ),
        hashKey(
          artistQueryKeys.showsPage(ARTIST_ID, {
            timeFilter: 'upcoming',
            limit: 20,
          }),
        ),
      ]),
    )
  })


  it('never sends an out-of-range year the backend would reject', async () => {
    const queryClient = createTestQueryClient()
    mockApiRequest.mockResolvedValue(twentyShows())

    const { result } = renderHook(
      () =>
        useArtistShows({
          artistId: ARTIST_ID,
          timeFilter: 'past',
          year: 1_759_000_000,
        }),
      { wrapper: createWrapperWithClient(queryClient) },
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest.mock.calls[0][0]).not.toContain('year=')
  })

  it('lands the past archive on the key artistPastShowsPageParams constructs', async () => {
    const queryClient = createTestQueryClient()
    mockApiRequest.mockResolvedValue(fiftyShows())

    // The hook builds its key from what it SENDS, and `artistPastShowsPageParams`
    // has to describe the same request — page 1's offset is 0, which the request
    // omits and the key must therefore record as "not sent". A drift throws
    // nothing; it just means anything seeding or reading that entry misses it.
    //
    // This used to guard a second reader: the archive PEEKED at neighbouring
    // pages' cache entries to label them. PSY-1842 replaced that with a month
    // histogram, so the normalization has one call site again — it is no less
    // load-bearing for it. Pinned on page 1 and on a later page.
    for (const [page, year] of [[1, null], [3, 2025]] as const) {
      const params = artistPastShowsPageParams(page, year)
      const { result } = renderHook(
        () =>
          useArtistShows({
            artistId: ARTIST_ID,
            timeFilter: 'past',
            limit: params.limit,
            offset: params.offset ?? 0,
            year: params.year,
          }),
        { wrapper: createWrapperWithClient(queryClient) },
      )
      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const peekKey = artistQueryKeys.showsPage(ARTIST_ID, params)
      expect(queryClient.getQueryData(peekKey)).toBeDefined()
      expect(
        queryClient
          .getQueryCache()
          .getAll()
          .some(entry => entry.queryHash === hashKey(peekKey)),
      ).toBe(true)
    }
  })
})
