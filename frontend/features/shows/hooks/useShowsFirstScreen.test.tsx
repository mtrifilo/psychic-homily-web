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
} from '@/features/shows/api'
import { useShowCities, useShowsCalendar } from './useShows'

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
})
