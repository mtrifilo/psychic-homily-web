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
  UPCOMING_SHOWS_FIRST_SCREEN_KEY,
  UPCOMING_SHOWS_FIRST_SCREEN_URL,
} from '@/features/shows/api'
import { useShowCities, useUpcomingShows } from './useShows'

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
 * "Bare `/shows`" now means literally that: no filters and NO ARGUMENTS. Since
 * PSY-1678 the request carries no per-viewer input at all, so the hooks are
 * invoked below exactly as `ShowList` invokes them on a cold anon load. That is
 * a stronger contract than the one this file could assert before, when the
 * canonical timezone had to be passed in by hand to stand in for what
 * `useBrowserTimezone` reported through hydration.
 */
describe('shows first-screen prefetch contract', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('useUpcomingShows requests UPCOMING_SHOWS_FIRST_SCREEN_URL and keys on UPCOMING_SHOWS_FIRST_SCREEN_KEY', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], pagination: {}, total: 0 })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(() => useUpcomingShows(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      UPCOMING_SHOWS_FIRST_SCREEN_URL,
      { method: 'GET' },
    )

    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    // Hash, not deep-equal: the hash is what TanStack matches a hydrated entry
    // by, so it is the equality that actually decides whether the seed lands.
    expect(cached[0].queryHash).toBe(hashKey(UPCOMING_SHOWS_FIRST_SCREEN_KEY))
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

  // The seeded entry has to be a HIT, not merely present: that is the whole
  // acceptance criterion of PSY-1678 (server HTML and post-hydration list are
  // the same rows, with no discarded refetch). Seeding the key the way the page
  // does and then mounting the hook has to serve the seeded rows without going
  // to the network at all.
  it('a seeded first-screen entry is served without a refetch', async () => {
    const seeded = {
      shows: [{ id: 1, title: 'Seeded Show' }],
      pagination: {},
      total: 1,
    }
    const queryClient = createTestQueryClient()
    queryClient.setQueryData(UPCOMING_SHOWS_FIRST_SCREEN_KEY, seeded)

    const { result } = renderHook(() => useUpcomingShows(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toEqual(seeded)
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  // The counterpart: a real filter still keys elsewhere, so the seed is a hit
  // for the canonical list and a miss for a filtered deep link — degraded,
  // never mismatched.
  it('a city filter moves the request off the first-screen entry', async () => {
    mockApiRequest.mockResolvedValue({ shows: [], pagination: {}, total: 0 })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(
      () => useUpcomingShows({ cities: [{ city: 'Phoenix', state: 'AZ' }] }),
      { wrapper: createWrapperWithClient(queryClient) },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(queryClient.getQueryCache().getAll()[0].queryHash).not.toBe(
      hashKey(UPCOMING_SHOWS_FIRST_SCREEN_KEY),
    )
  })
})
