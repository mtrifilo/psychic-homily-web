import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { hashKey } from '@tanstack/react-query'
import { createTestQueryClient, createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

// Deliberately NOT mocking '@/features/venues/api': the whole point of these
// tests is the REAL query key. The sibling useVenues.test.tsx mocks that
// module, so it cannot see this class of bug at all.
import {
  VENUE_SHOWS_PAGE_LIMIT,
  VENUE_SHOWS_VIEWER_TIMEZONE,
  venuePastShowsPageParams,
  venueQueryKeys,
} from '@/features/venues/api'
import { useVenueShows } from './useVenues'

const VENUE_ID = 42

/** The compact page VenueCard's expanded preview asks for. */
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
 * PSY-1698. `venueQueryKeys.shows()` used to be the whole cache key for a
 * venue-shows request, keyed on venue + time filter while `limit` and
 * `timezone` varied per caller. Every venue-shows surface therefore shared one
 * entry regardless of what it had actually asked for, and whichever request
 * resolved first answered for all of them for the full 5-minute staleTime.
 *
 * The live instance: `VenueCard`'s expanded preview (20 rows) and the venue
 * page's shows list (50 rows at the time) both keyed on the numeric venue id,
 * so expanding a card on /venues and then opening that venue handed the venue
 * page the 20-row list. Which list you got depended purely on the order you
 * navigated in, which is why this has to be asserted rather than reviewed.
 *
 * PSY-1753 widened the key again, with `year` and `offset`, and added a second
 * way to get it wrong: the past archive both ISSUES a page request and PEEKS
 * at the cache for its neighbours, through two different code paths that must
 * agree on the key exactly. The last case here pins them together.
 */
describe('venue-shows cache key isolates differently-parameterized callers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('does not let a 20-row preview answer the venue page for the same venue', async () => {
    const queryClient = createTestQueryClient()
    const wrapper = createWrapperWithClient(queryClient)

    // The preview lands FIRST — the navigation order that used to poison the
    // venue page's entry.
    mockApiRequest.mockResolvedValueOnce(twentyShows())
    const preview = renderHook(
      () =>
        useVenueShows({
          venueId: VENUE_ID,
          limit: PREVIEW_LIMIT,
          timezone: VENUE_SHOWS_VIEWER_TIMEZONE,
        }),
      { wrapper },
    )
    await waitFor(() => expect(preview.result.current.isSuccess).toBe(true))

    // Then the venue page mounts against the same warm cache.
    mockApiRequest.mockResolvedValueOnce(fiftyShows())
    const venuePage = renderHook(
      () =>
        useVenueShows({
          venueId: VENUE_ID,
          limit: VENUE_SHOWS_PAGE_LIMIT,
          timezone: VENUE_SHOWS_VIEWER_TIMEZONE,
          timeFilter: 'upcoming',
        }),
      { wrapper },
    )
    await waitFor(() => expect(venuePage.result.current.isSuccess).toBe(true))

    // The regression: before the fix this was 20, served from the preview's
    // entry without ever issuing the venue page's own request.
    expect(venuePage.result.current.data?.shows).toHaveLength(
      VENUE_SHOWS_PAGE_LIMIT,
    )
    expect(preview.result.current.data?.shows).toHaveLength(PREVIEW_LIMIT)

    // Two questions, two requests, two entries.
    expect(mockApiRequest).toHaveBeenCalledTimes(2)
    expect(queryClient.getQueryCache().getAll()).toHaveLength(2)
  })

  it('keys the request on limit, timezone and time filter', async () => {
    const queryClient = createTestQueryClient()
    mockApiRequest.mockResolvedValue(fiftyShows())

    const { result } = renderHook(
      () =>
        useVenueShows({
          venueId: VENUE_ID,
          limit: VENUE_SHOWS_PAGE_LIMIT,
          timezone: 'America/Phoenix',
          timeFilter: 'upcoming',
        }),
      { wrapper: createWrapperWithClient(queryClient) },
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    expect(cached[0].queryHash).toBe(
      hashKey(
        venueQueryKeys.showsPage(VENUE_ID, {
          timeFilter: 'upcoming',
          limit: VENUE_SHOWS_PAGE_LIMIT,
          timezone: 'America/Phoenix',
        }),
      ),
    )
  })

  it('still nests every page under the shows() invalidation prefix', async () => {
    const queryClient = createTestQueryClient()
    mockApiRequest.mockResolvedValue(fiftyShows())

    const wrapper = createWrapperWithClient(queryClient)
    const upcoming = renderHook(
      () =>
        useVenueShows({
          venueId: VENUE_ID,
          limit: VENUE_SHOWS_PAGE_LIMIT,
          timezone: VENUE_SHOWS_VIEWER_TIMEZONE,
          timeFilter: 'upcoming',
        }),
      { wrapper },
    )
    const past = renderHook(
      () =>
        useVenueShows({
          venueId: VENUE_ID,
          limit: PREVIEW_LIMIT,
          timezone: VENUE_SHOWS_VIEWER_TIMEZONE,
          timeFilter: 'past',
        }),
      { wrapper },
    )
    await waitFor(() => expect(upcoming.result.current.isSuccess).toBe(true))
    await waitFor(() => expect(past.result.current.isSuccess).toBe(true))

    // Widening the key must not narrow invalidation: createInvalidateQueries
    // fans out from ['venues'], and mutations reach one venue's pages via
    // shows(). Both survive only while the extra segments are appended.
    const matched = queryClient
      .getQueryCache()
      .findAll({ queryKey: venueQueryKeys.shows(VENUE_ID) })
    expect(matched).toHaveLength(2)
    expect(
      queryClient.getQueryCache().findAll({ queryKey: ['venues'] }),
    ).toHaveLength(2)
  })

  it('keys on what the URL actually sent, not on the raw argument', async () => {
    const queryClient = createTestQueryClient()
    mockApiRequest.mockResolvedValue(twentyShows())
    const wrapper = createWrapperWithClient(queryClient)

    // A falsy limit and an empty timezone both drop out of the URL, so these
    // two hooks issue the SAME request. Keying on the raw argument would mint
    // two entries for it — the mirror image of the collision above, and the
    // reason the hook resolves the sent values once for both.
    const zeroLimit = renderHook(
      () => useVenueShows({ venueId: VENUE_ID, limit: 0, timezone: '' }),
      { wrapper },
    )
    const omitted = renderHook(
      () => useVenueShows({ venueId: VENUE_ID, limit: undefined }),
      { wrapper },
    )
    await waitFor(() => expect(zeroLimit.result.current.isSuccess).toBe(true))
    await waitFor(() => expect(omitted.result.current.isSuccess).toBe(true))

    // `limit: undefined` takes the hook's own default of 20 and sends it;
    // `limit: 0` sends nothing. Different requests, so still two entries --
    // but each entry matches the URL that filled it.
    const urls = mockApiRequest.mock.calls.map(c => c[0] as string)
    expect(urls.some(u => !u.includes('limit='))).toBe(true)
    expect(urls.some(u => u.includes('limit=20'))).toBe(true)
    expect(urls.every(u => !u.includes('timezone='))).toBe(true)
  })

  it('separates a caller that omits the timezone from one that sends it', async () => {
    const queryClient = createTestQueryClient()
    mockApiRequest.mockResolvedValue(twentyShows())
    const wrapper = createWrapperWithClient(queryClient)

    // An omitted timezone is not the same question: the backend falls back to
    // UTC, which puts the upcoming/past boundary somewhere else entirely.
    const withZone = renderHook(
      () =>
        useVenueShows({
          venueId: VENUE_ID,
          limit: PREVIEW_LIMIT,
          timezone: 'America/Phoenix',
        }),
      { wrapper },
    )
    const withoutZone = renderHook(
      () => useVenueShows({ venueId: VENUE_ID, limit: PREVIEW_LIMIT }),
      { wrapper },
    )
    await waitFor(() => expect(withZone.result.current.isSuccess).toBe(true))
    await waitFor(() => expect(withoutZone.result.current.isSuccess).toBe(true))

    expect(queryClient.getQueryCache().getAll()).toHaveLength(2)
  })

  it('lands the past archive on the key its own neighbour peek constructs', async () => {
    const queryClient = createTestQueryClient()
    mockApiRequest.mockResolvedValue(fiftyShows())

    // `VenuePastShows` labels each page button with the months it covers by
    // reading sibling pages straight out of the cache, keyed from
    // `venuePastShowsPageParams`. If the key the hook registers and the key the
    // peek builds ever drift, nothing throws: the labels just silently stop
    // appearing. Both paths are pinned here, on page 1 (where offset is 0 and
    // must be keyed as "not sent") and on a later page.
    for (const [page, year] of [[1, null], [3, 2025]] as const) {
      const params = venuePastShowsPageParams(page, year)
      const { result } = renderHook(
        () =>
          useVenueShows({
            venueId: VENUE_ID,
            timeFilter: 'past',
            timezone: params.timezone,
            limit: params.limit,
            offset: params.offset ?? 0,
            year: params.year,
          }),
        { wrapper: createWrapperWithClient(queryClient) },
      )
      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const peekKey = venueQueryKeys.showsPage(VENUE_ID, params)
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
