import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { hashKey } from '@tanstack/react-query'
import { createTestQueryClient, createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

// Deliberately NOT mocking '@/features/venues/api' — see the sibling
// useShowsFirstScreen.test.tsx for why.
import {
  VENUE_LIST_FIRST_SCREEN_KEY,
  VENUE_LIST_FIRST_SCREEN_URL,
  VENUE_LIST_PAGE_LIMIT,
  venueQueryKeys,
} from '@/features/venues/api'
import { useVenueCities, useVenues } from './useVenues'

/**
 * `app/venues/page.tsx` server-renders the first screen by fetching
 * `VENUE_LIST_FIRST_SCREEN_URL` and seeding `VENUE_LIST_FIRST_SCREEN_KEY`
 * (PSY-1624). A drift between those constants and what `VenueList`'s hooks
 * actually request degrades silently to the pre-SSR behaviour, so it has to be
 * asserted rather than reviewed.
 */
describe('venues first-screen prefetch contract', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('useVenues requests VENUE_LIST_FIRST_SCREEN_URL and keys on VENUE_LIST_FIRST_SCREEN_KEY', async () => {
    mockApiRequest.mockResolvedValueOnce({ venues: [], total: 0 })
    const queryClient = createTestQueryClient()

    // The arguments VenueList passes on a bare /venues: its page size, the
    // first page, and the default AND tag semantics.
    const { result } = renderHook(
      () =>
        useVenues({
          cities: undefined,
          tags: undefined,
          tagMatch: 'all',
          limit: VENUE_LIST_PAGE_LIMIT,
          offset: 0,
        }),
      { wrapper: createWrapperWithClient(queryClient) },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(VENUE_LIST_FIRST_SCREEN_URL, {
      method: 'GET',
    })

    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    expect(cached[0].queryHash).toBe(hashKey(VENUE_LIST_FIRST_SCREEN_KEY))
  })

  it('useVenueCities keys on venueQueryKeys.cities', async () => {
    mockApiRequest.mockResolvedValueOnce({ cities: [] })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(() => useVenueCities(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    expect(cached[0].queryHash).toBe(hashKey(venueQueryKeys.cities))
  })

  it('a city filter moves the request off the first-screen entry', async () => {
    mockApiRequest.mockResolvedValue({ venues: [], total: 0 })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(
      () =>
        useVenues({
          cities: [{ city: 'Phoenix', state: 'AZ' }],
          tagMatch: 'all',
          limit: VENUE_LIST_PAGE_LIMIT,
          offset: 0,
        }),
      { wrapper: createWrapperWithClient(queryClient) },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // A filtered deep link is out of scope for the seed on purpose: it misses,
    // and both render passes agree on the spinner. Degraded, never mismatched.
    expect(queryClient.getQueryCache().getAll()[0].queryHash).not.toBe(
      hashKey(VENUE_LIST_FIRST_SCREEN_KEY),
    )
  })
})
