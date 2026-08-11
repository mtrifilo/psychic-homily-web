import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { hashKey } from '@tanstack/react-query'
import { createTestQueryClient, createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

// Deliberately NOT mocking '@/features/artists/api' — see the sibling
// useVenuesFirstScreen.test.tsx for why.
import {
  ARTIST_LIST_FIRST_SCREEN_KEY,
  ARTIST_LIST_FIRST_SCREEN_URL,
  ARTIST_LIST_PAGE_LIMIT,
  artistQueryKeys,
} from '@/features/artists/api'
import { useArtistCities, useArtists } from './useArtists'

/**
 * `app/artists/page.tsx` server-renders the first screen by fetching
 * `ARTIST_LIST_FIRST_SCREEN_URL` and seeding `ARTIST_LIST_FIRST_SCREEN_KEY`
 * (PSY-1774). A drift between those constants and what `ArtistList`'s hooks
 * actually request degrades silently to the pre-SSR behaviour, so it has to be
 * asserted rather than reviewed.
 */
describe('artists first-screen prefetch contract', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('useArtists requests ARTIST_LIST_FIRST_SCREEN_URL and keys on ARTIST_LIST_FIRST_SCREEN_KEY', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [], total: 0, limit: 50, offset: 0 })
    const queryClient = createTestQueryClient()

    // The arguments ArtistList passes on a bare /artists: its page size, the
    // first page, and the default AND tag semantics.
    const { result } = renderHook(
      () =>
        useArtists({
          cities: undefined,
          tags: undefined,
          tagMatch: 'all',
          limit: ARTIST_LIST_PAGE_LIMIT,
          offset: 0,
        }),
      { wrapper: createWrapperWithClient(queryClient) },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(ARTIST_LIST_FIRST_SCREEN_URL, {
      method: 'GET',
    })

    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    expect(cached[0].queryHash).toBe(hashKey(ARTIST_LIST_FIRST_SCREEN_KEY))
  })

  it('useArtistCities keys on artistQueryKeys.cities', async () => {
    mockApiRequest.mockResolvedValueOnce({ cities: [] })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(() => useArtistCities(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const cached = queryClient.getQueryCache().getAll()
    expect(cached).toHaveLength(1)
    expect(cached[0].queryHash).toBe(hashKey(artistQueryKeys.cities))
  })

  it('a city filter moves the request off the first-screen entry', async () => {
    mockApiRequest.mockResolvedValue({ artists: [], total: 0, limit: 50, offset: 0 })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(
      () =>
        useArtists({
          cities: [{ city: 'Phoenix', state: 'AZ' }],
          tagMatch: 'all',
          limit: ARTIST_LIST_PAGE_LIMIT,
          offset: 0,
        }),
      { wrapper: createWrapperWithClient(queryClient) },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // A filtered deep link is out of scope for the seed on purpose: it misses,
    // and both render passes agree on the spinner. Degraded, never mismatched.
    expect(queryClient.getQueryCache().getAll()[0].queryHash).not.toBe(
      hashKey(ARTIST_LIST_FIRST_SCREEN_KEY),
    )
  })

  it('a deep page moves the request off the first-screen entry', async () => {
    mockApiRequest.mockResolvedValue({ artists: [], total: 0, limit: 50, offset: 50 })
    const queryClient = createTestQueryClient()

    const { result } = renderHook(
      () =>
        useArtists({
          tagMatch: 'all',
          limit: ARTIST_LIST_PAGE_LIMIT,
          offset: ARTIST_LIST_PAGE_LIMIT,
        }),
      { wrapper: createWrapperWithClient(queryClient) },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Same reasoning as the filtered case, and the one that would bite hardest:
    // page 2 reading page 1's seed would render the wrong 50 artists under a
    // `?page=2` URL, which is a WRONG answer rather than a degraded one.
    expect(queryClient.getQueryCache().getAll()[0].queryHash).not.toBe(
      hashKey(ARTIST_LIST_FIRST_SCREEN_KEY),
    )
  })
})
