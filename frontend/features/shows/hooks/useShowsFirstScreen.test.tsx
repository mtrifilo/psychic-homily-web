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
import { CANONICAL_FIRST_SCREEN_TIMEZONE } from '@/lib/canonicalTimezone'
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
 * "Bare `/shows`" means no filters, and the canonical timezone rather than the
 * viewer's, because that is what `useBrowserTimezone` reports through the
 * hydration render. Passing it explicitly here is the point: these hooks must
 * land on the seeded entry when handed exactly what that render hands them.
 */
describe('shows first-screen prefetch contract', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('useUpcomingShows requests UPCOMING_SHOWS_FIRST_SCREEN_URL and keys on UPCOMING_SHOWS_FIRST_SCREEN_KEY', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], pagination: {}, total: 0 })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(
      () => useUpcomingShows({ timezone: CANONICAL_FIRST_SCREEN_TIMEZONE }),
      { wrapper: createWrapperWithClient(queryClient) },
    )

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

    const { result } = renderHook(
      () => useShowCities({ timezone: CANONICAL_FIRST_SCREEN_TIMEZONE }),
      { wrapper: createWrapperWithClient(queryClient) },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(SHOW_CITIES_FIRST_SCREEN_URL, {
      method: 'GET',
    })

    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    expect(cached[0].queryHash).toBe(hashKey(SHOW_CITIES_FIRST_SCREEN_KEY))
  })

  it('a DIFFERENT timezone moves the request off the first-screen entry', async () => {
    mockApiRequest.mockResolvedValue({ shows: [], pagination: {}, total: 0 })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(
      () => useUpcomingShows({ timezone: 'America/New_York' }),
      { wrapper: createWrapperWithClient(queryClient) },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // The counterpart of the two tests above, and the reason the canonical zone
    // has to be a shared constant rather than a value each side picks. This is
    // the post-hydration refinement: a real viewer zone keys elsewhere, which
    // is correct. It would also be what the SERVER produced if it read `Intl`
    // directly, and then the server HTML and the hydration render would
    // disagree.
    expect(queryClient.getQueryCache().getAll()[0].queryHash).not.toBe(
      hashKey(UPCOMING_SHOWS_FIRST_SCREEN_KEY),
    )
  })
})
