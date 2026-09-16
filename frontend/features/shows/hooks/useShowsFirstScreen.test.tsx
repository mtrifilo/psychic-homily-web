import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { hashKey } from '@tanstack/react-query'
import { createTestQueryClient, createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

// Deliberately NOT mocking '@/features/shows/api' — the whole point of this
// file is to check the REAL constants against what the REAL hooks do. The
// sibling useShows.test.tsx mocks that module to test request-building in
// isolation, which is exactly why it cannot catch the drift below.
import {
  SHOW_CITIES_FIRST_SCREEN_KEY,
  SHOW_CITIES_FIRST_SCREEN_URL,
  SHOWS_CALENDAR_FIRST_SCREEN_KEY,
  SHOWS_CALENDAR_FIRST_SCREEN_URL,
  SHOWS_MONTHS_FIRST_SCREEN_KEY,
  SHOWS_MONTHS_FIRST_SCREEN_URL,
  showCitiesWindowFirstScreenKey,
  showCitiesWindowFirstScreenUrl,
  showsCalendarWindowFirstScreenKey,
  showsCalendarWindowFirstScreenUrl,
} from '@/features/shows/api'
import { useShowCities, useShowMonths, useShowsCalendar } from './useShows'

/**
 * `app/shows/page.tsx` server-renders the first screen by fetching
 * `*_FIRST_SCREEN_URL` and seeding `*_FIRST_SCREEN_KEY` (PSY-1624). That only
 * works while the constants describe what `ShowList`'s hooks actually do on a
 * bare `/shows`, and nothing enforces it: a drifted key seeds a cache entry
 * the hook never reads, so the hook falls back to fetching, both render passes
 * agree on the skeleton, and the page silently stops being server-rendered
 * with no error anywhere. These tests are the only thing standing between that
 * regression and production.
 *
 * "Bare `/shows`" means page 1 with no filters: NO ARGUMENTS, and the only
 * parameter on the wire is the page size, which the list states rather than
 * inheriting. The request carries no per-viewer input at all, so the hooks are
 * invoked below exactly as `ShowList` invokes them on a cold anon load.
 */
describe('shows first-screen prefetch contract', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('useShowsCalendar requests SHOWS_CALENDAR_FIRST_SCREEN_URL and keys on SHOWS_CALENDAR_FIRST_SCREEN_KEY', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], pagination: {}, total: 0 })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(() => useShowsCalendar(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      SHOWS_CALENDAR_FIRST_SCREEN_URL,
      { method: 'GET' },
    )

    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    // Hash, not deep-equal: the hash is what TanStack matches a hydrated entry
    // by, so it is the equality that actually decides whether the seed lands.
    expect(cached[0].queryHash).toBe(hashKey(SHOWS_CALENDAR_FIRST_SCREEN_KEY))
  })

  it('useShowCities requests SHOW_CITIES_FIRST_SCREEN_URL and keys on SHOW_CITIES_FIRST_SCREEN_KEY', async () => {
    mockApiRequest.mockResolvedValueOnce({ cities: [] })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(() => useShowCities(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(SHOW_CITIES_FIRST_SCREEN_URL, {
      method: 'GET',
    })

    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    expect(cached[0].queryHash).toBe(hashKey(SHOW_CITIES_FIRST_SCREEN_KEY))
  })

  it('an empty scope is the same request and the same entry as no scope', async () => {
    mockApiRequest.mockResolvedValueOnce({ cities: [] })
    const queryClient = createTestQueryClient()

    // What ShowList passes on a bare /shows: no tags, no window, default AND.
    const { result } = renderHook(
      () => useShowCities({ tags: undefined, tagMatch: 'all', window: undefined }),
      { wrapper: createWrapperWithClient(queryClient) }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(SHOW_CITIES_FIRST_SCREEN_URL, {
      method: 'GET',
    })
    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    expect(cached[0].queryHash).toBe(hashKey(SHOW_CITIES_FIRST_SCREEN_KEY))
  })

  it('a tag filter and a window each move the facet off the unscoped entry', async () => {
    mockApiRequest.mockResolvedValue({ cities: [] })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(
      () =>
        useShowCities({
          tags: ['post-punk'],
          tagMatch: 'all',
          window: { year: 2026, month: 9, day: 15, days: 3 },
        }),
      { wrapper: createWrapperWithClient(queryClient) }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // The window params are spelled by the same builder the list's own request
    // uses, so the facet and the list cannot address different windows.
    expect(mockApiRequest).toHaveBeenCalledWith(
      `${SHOW_CITIES_FIRST_SCREEN_URL}?tags=post-punk&year=2026&month=9&day=15&days=3`,
      { method: 'GET' }
    )
    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    expect(cached[0].queryHash).not.toBe(hashKey(SHOW_CITIES_FIRST_SCREEN_KEY))
  })

  // The seeded entry has to be a HIT — the hook must PAINT the server's rows
  // rather than fall through to a loading state and fetch them again.
  //
  // Seeded at `updatedAt: 0`, which is what `seedFirstScreen` actually writes,
  // so this reproduces production rather than a friendlier version of it. That
  // distinction matters: seeding without it would make the entry fresh, suppress
  // the revalidation, and let this test keep passing even if the real path
  // degraded to a full miss. What production does is serve the seeded rows
  // immediately AND revalidate the same key once, which is deliberate — the
  // server fetch forwards no cookies, so the seed is always the anonymous
  // payload and an admin's unapproved shows arrive only on that refetch (see
  // `lib/query-hydration.ts`). So the property to pin is "data is present on the
  // first commit, and any request that does go out is THIS key's", not "no
  // request at all".
  it('serves the seeded first-screen rows immediately, and only revalidates the same key', async () => {
    const seeded = {
      shows: [{ id: 1, title: 'Seeded Show' }],
      pagination: {},
      total: 1,
    }
    mockApiRequest.mockResolvedValue(seeded)
    const queryClient = createTestQueryClient()
    queryClient.setQueryData(SHOWS_CALENDAR_FIRST_SCREEN_KEY, seeded, {
      updatedAt: 0,
    })

    const { result } = renderHook(() => useShowsCalendar(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    // Present on the very first commit: no loading state, no waiting.
    expect(result.current.data).toEqual(seeded)

    await waitFor(() => expect(result.current.isFetching).toBe(false))

    // The revalidation, if it ran, went to the first-screen URL and landed back
    // on the first-screen key. A second, differently-keyed request is the
    // regression this guards (it is what the viewer-timezone parameter caused).
    for (const call of mockApiRequest.mock.calls) {
      expect(call[0]).toBe(SHOWS_CALENDAR_FIRST_SCREEN_URL)
    }
    expect(queryClient.getQueryCache().getAll()).toHaveLength(1)
    expect(result.current.data).toEqual(seeded)
  })

  // Neither the URL nor the key may carry a viewer zone. The KEY is what decides
  // whether the server-seeded entry is a hit, so a timezone there would
  // re-fragment the cache per viewer and undo PSY-1678. A timezone in the URL
  // alone would not break the seed at all — that is exactly why the deleted
  // param could be a FIXED zone — but it is the affordance a future reader
  // copies back into the key, which is the cost worth asserting against. The
  // URLs are asserted BARE so the constants stay byte-identical to what the
  // hooks request; the test above is what pins that pairing.
  it('carries no viewer zone in either the URL or the key', () => {
    // The page size is the ONLY parameter the seeded list URL may carry, and no
    // `offset`: page 1 is the seeded page.
    expect(SHOWS_CALENDAR_FIRST_SCREEN_URL).toContain('?limit=50')
    expect(SHOWS_CALENDAR_FIRST_SCREEN_URL).not.toContain('offset')
    expect(SHOWS_CALENDAR_FIRST_SCREEN_URL).not.toContain('timezone')
    expect(SHOW_CITIES_FIRST_SCREEN_URL).not.toContain('?')
    expect(JSON.stringify(SHOWS_CALENDAR_FIRST_SCREEN_KEY)).not.toContain(
      'timezone'
    )
    expect(JSON.stringify(SHOWS_CALENDAR_FIRST_SCREEN_KEY)).not.toContain(
      'America'
    )
    expect(JSON.stringify(SHOW_CITIES_FIRST_SCREEN_KEY)).not.toContain(
      'timezone'
    )
  })

  // The counterpart: a real filter still keys elsewhere, so the seed is a hit
  // for the canonical list and a miss for a filtered deep link — degraded,
  // never mismatched.
  it('a city filter moves the request off the first-screen entry', async () => {
    mockApiRequest.mockResolvedValue({ shows: [], pagination: {}, total: 0 })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(
      () => useShowsCalendar({ cities: [{ city: 'Phoenix', state: 'AZ' }] }),
      { wrapper: createWrapperWithClient(queryClient) },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(queryClient.getQueryCache().getAll()[0].queryHash).not.toBe(
      hashKey(SHOWS_CALENDAR_FIRST_SCREEN_KEY),
    )
  })

  // The histogram is the THIRD call this route would otherwise make on a cold
  // anonymous view, against a per-IP budget everyone behind one address shares.
  // Its pairing has to hold for the same reason the other two do.
  it('useShowMonths requests SHOWS_MONTHS_FIRST_SCREEN_URL and keys on SHOWS_MONTHS_FIRST_SCREEN_KEY', async () => {
    mockApiRequest.mockResolvedValueOnce({ months: [], total: 0 })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(() => useShowMonths(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      SHOWS_MONTHS_FIRST_SCREEN_URL,
      { method: 'GET' },
    )

    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    expect(cached[0].queryHash).toBe(hashKey(SHOWS_MONTHS_FIRST_SCREEN_KEY))
  })
})

/**
 * The same contract for the DATE-ADDRESSED lists (PSY-2061). The window routes
 * seed `showsCalendarWindowFirstScreenKey(window)` from
 * `showsCalendarWindowFirstScreenUrl(window)`, and the pair has to describe
 * what `ShowList` asks for on page 1 of that window or the month page quietly
 * stops being server-rendered, the same silent regression, with no error
 * anywhere, that the root's contract above exists to prevent.
 */
describe('shows window first-screen prefetch contract', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it.each([
    ['a month', { year: 2026, month: 11 }],
    ['a day', { year: 2026, month: 11, day: 4 }],
    ['a single-digit month', { year: 2026, month: 9 }],
  ])('useShowsCalendar pairs the URL and key for %s', async (_label, window) => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(() => useShowsCalendar({ window }), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      showsCalendarWindowFirstScreenUrl(window),
      { method: 'GET' }
    )

    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    expect(cached[0].queryHash).toBe(
      hashKey(showsCalendarWindowFirstScreenKey(window))
    )
  })

  /**
   * A window entry and the root entry must never be the same entry: seeding one
   * as the other would serve a month's rows as the whole upcoming list, or the
   * reverse, and look like a cache hit either way.
   */
  it('keys a window apart from the root and from its own sibling windows', () => {
    const month = hashKey(showsCalendarWindowFirstScreenKey({ year: 2026, month: 11 }))
    const day = hashKey(
      showsCalendarWindowFirstScreenKey({ year: 2026, month: 11, day: 4 })
    )
    const otherMonth = hashKey(
      showsCalendarWindowFirstScreenKey({ year: 2026, month: 12 })
    )

    expect(new Set([month, day, otherMonth, hashKey(SHOWS_CALENDAR_FIRST_SCREEN_KEY)]).size).toBe(4)
  })

  /**
   * The unwindowed key is unchanged by windows existing at all: react-query
   * hashes through `JSON.stringify`, which drops the undefined members the
   * window contributes to a filterless call.
   */
  it('leaves the root entry where it was', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(() => useShowsCalendar(), {
      wrapper: createWrapperWithClient(queryClient),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(queryClient.getQueryCache().getAll()[0].queryHash).toBe(
      hashKey(SHOWS_CALENDAR_FIRST_SCREEN_KEY)
    )
  })

  /**
   * The city facet's half of the same contract. A windowed route seeds a facet
   * scoped to its own window, so the seed pair has to address the entry
   * `ShowList` reads there — otherwise the route pays for a payload nothing
   * reads AND a client round trip for the one it does.
   */
  it('seeds the windowed facet on the entry the windowed list reads', async () => {
    const window = { year: 2026, month: 12, day: 3 }
    mockApiRequest.mockResolvedValueOnce({ cities: [] })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(() => useShowCities({ window }), {
      wrapper: createWrapperWithClient(queryClient),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      showCitiesWindowFirstScreenUrl(window),
      { method: 'GET' }
    )
    expect(queryClient.getQueryCache().getAll()[0].queryHash).toBe(
      hashKey(showCitiesWindowFirstScreenKey(window))
    )
  })

  // An undefined window is the root's pair, so the root page's constants and
  // the windowed builders cannot describe two different unwindowed entries.
  it('collapses the windowed facet pair to the root pair with no window', () => {
    expect(showCitiesWindowFirstScreenUrl(undefined)).toBe(
      SHOW_CITIES_FIRST_SCREEN_URL
    )
    expect(hashKey(showCitiesWindowFirstScreenKey(undefined))).toBe(
      hashKey(SHOW_CITIES_FIRST_SCREEN_KEY)
    )
  })
})
