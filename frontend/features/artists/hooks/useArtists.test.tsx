import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import {
  createWrapper,
  createWrapperWithClient,
  createTestQueryClient,
} from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock the feature api module
vi.mock('@/features/artists/api', () => ({
  // Mirrors the real constant. This file mocks the api module, so it cannot
  // assert the real first-screen pair — useArtistsFirstScreen.test.tsx does
  // that against the genuine constants.
  ARTIST_LIST_PAGE_LIMIT: 50,
  artistEndpoints: {
    LIST: '/artists',
    CITIES: '/artists/cities',
    GET: (artistId: string | number) => `/artists/${artistId}`,
    SHOWS: (artistId: string | number) => `/artists/${artistId}/shows`,
    SHOW_YEARS: (artistId: string | number) => `/artists/${artistId}/shows/years`,
  },
  artistQueryKeys: {
    list: (filters?: Record<string, unknown>) => ['artists', 'list', filters],
    cities: ['artists', 'cities'],
    detail: (id: string | number) => ['artists', 'detail', String(id)],
    shows: (artistId: string | number) => ['artists', 'shows', String(artistId)],
    // Mirrors the real builder. This file mocks the api module, so it cannot
    // assert the real key — useArtistShowsCacheKey.test.tsx does that against
    // the genuine one.
    showsPage: (
      artistId: string | number,
      params: {
        timeFilter: string
        limit?: number
        year?: number
        offset?: number
      },
    ) => [
      'artists',
      'shows',
      String(artistId),
      params.timeFilter,
      params.limit ?? null,
      params.year ?? null,
      params.offset ?? null,
    ],
    showYears: (artistId: string | number, timeFilter: string) => [
      'artists',
      'shows',
      String(artistId),
      'years',
      timeFilter,
    ],
  },
}))

// Import hooks after mocks are set up
import { useArtists, useArtistCities, useArtist, useArtistShows } from './useArtists'


describe('useArtists', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('useArtist', () => {
    it('fetches a single artist by ID', async () => {
      const mockArtist = {
        id: 1,
        name: 'Test Artist',
        social: {
          bandcamp: 'https://testartist.bandcamp.com',
          spotify: null as string | null,
        },
      }
      mockApiRequest.mockResolvedValueOnce(mockArtist)

      const { result } = renderHook(() => useArtist({ artistId: 1 }), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/artists/1', {
        method: 'GET',
      })
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () => useArtist({ artistId: 1, enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when artistId is 0 or negative', async () => {
      const { result: result0 } = renderHook(
        () => useArtist({ artistId: 0 }),
        { wrapper: createWrapper() }
      )

      const { result: resultNeg } = renderHook(
        () => useArtist({ artistId: -1 }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result0.current.fetchStatus).toBe('idle')
      expect(resultNeg.current.fetchStatus).toBe('idle')
    })

    it('handles artist not found error', async () => {
      const error = new Error('Artist not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useArtist({ artistId: 999 }), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Artist not found')
    })

  })

  describe('useArtists', () => {
    // The default page size is ALWAYS sent, including when the caller names no
    // filters. An unbounded request is the defect PSY-1774 removed. The zero
    // offset is asserted here too: it is omitted from the URL but kept in the
    // key, and `ARTIST_LIST_FIRST_SCREEN_URL` has to equal exactly this string.
    it('bounds an unfiltered request and omits the zero offset', async () => {
      const mockResponse = {
        artists: [{ id: 1, name: 'Artist A' }, { id: 2, name: 'Artist B' }],
        total: 2,
        limit: 50,
        offset: 0,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useArtists(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/artists?limit=50', { method: 'GET' })
    })

    it('includes cities filter in query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ artists: [], total: 0, limit: 50, offset: 0 })

      const { result } = renderHook(
        () => useArtists({ cities: [{ city: 'Phoenix', state: 'AZ' }, { city: 'Mesa', state: 'AZ' }] }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/artists?cities=Phoenix%2CAZ%7CMesa%2CAZ&limit=50',
        { method: 'GET' }
      )
    })

    it('sends a non-zero offset and keys pages apart', async () => {
      mockApiRequest.mockResolvedValue({ artists: [], total: 120, limit: 50, offset: 50 })
      const queryClient = createTestQueryClient()

      const { result } = renderHook(() => useArtists({ offset: 50 }), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/artists?limit=50&offset=50', {
        method: 'GET',
      })

      // Page 2 must not answer for page 1: the offset is in the key, so the
      // two land on different entries rather than one silently serving both.
      const { result: firstPage } = renderHook(() => useArtists(), {
        wrapper: createWrapperWithClient(queryClient),
      })
      await waitFor(() => expect(firstPage.current.isSuccess).toBe(true))
      expect(queryClient.getQueryCache().getAll()).toHaveLength(2)
    })
  })

  describe('useArtistCities', () => {
    it('fetches artist cities', async () => {
      const mockResponse = {
        cities: [
          { city: 'Phoenix', state: 'AZ', artist_count: 10 },
          { city: 'Mesa', state: 'AZ', artist_count: 5 },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useArtistCities(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/artists/cities', { method: 'GET' })
    })

  })

  describe('useArtistShows', () => {
    it('fetches shows for an artist with default options', async () => {
      const mockResponse = {
        shows: [
          { id: 1, title: 'Show 1' },
          { id: 2, title: 'Show 2' },
        ],
        total: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useArtistShows({ artistId: 1 }), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      // Default time_filter is 'upcoming', default limit is 20
      expect(mockApiRequest).toHaveBeenCalledWith(
        '/artists/1/shows?limit=20&time_filter=upcoming',
        { method: 'GET' }
      )
    })

    it('supports upcoming time filter', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () => useArtistShows({ artistId: 1, timeFilter: 'upcoming' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).toContain('time_filter=upcoming')
    })

    it('supports past time filter', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () => useArtistShows({ artistId: 1, timeFilter: 'past' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).toContain('time_filter=past')
    })

    it('supports all time filter', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () => useArtistShows({ artistId: 1, timeFilter: 'all' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).toContain('time_filter=all')
    })

    it('supports custom limit', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () => useArtistShows({ artistId: 1, limit: 50 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).toContain('limit=50')
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () => useArtistShows({ artistId: 1, enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when artistId is invalid', async () => {
      const { result } = renderHook(
        () => useArtistShows({ artistId: 0 }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('combines multiple query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () =>
          useArtistShows({
            artistId: 5,
            limit: 10,
            timeFilter: 'past',
          }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('limit=10')
      expect(calledUrl).toContain('time_filter=past')
    })
  })
})
