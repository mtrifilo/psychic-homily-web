import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock the feature api module
vi.mock('@/features/artists/api', () => ({
  artistEndpoints: {
    LIST: '/artists',
    CITIES: '/artists/cities',
    GET: (artistId: string | number) => `/artists/${artistId}`,
    SHOWS: (artistId: string | number) => `/artists/${artistId}/shows`,
  },
  artistQueryKeys: {
    list: (filters?: Record<string, unknown>) => ['artists', 'list', filters],
    cities: ['artists', 'cities'],
    detail: (id: string | number) => ['artists', 'detail', String(id)],
    shows: (artistId: string | number) => ['artists', 'shows', String(artistId)],
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
          spotify: null,
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
    it('fetches all artists without filters', async () => {
      const mockResponse = {
        artists: [{ id: 1, name: 'Artist A' }, { id: 2, name: 'Artist B' }],
        count: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useArtists(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/artists', { method: 'GET' })
    })

    it('includes cities filter in query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ artists: [], count: 0 })

      const { result } = renderHook(
        () => useArtists({ cities: [{ city: 'Phoenix', state: 'AZ' }, { city: 'Mesa', state: 'AZ' }] }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/artists?cities=Phoenix%2CAZ%7CMesa%2CAZ',
        { method: 'GET' }
      )
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

    it('includes timezone in query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () => useArtistShows({ artistId: 1, timezone: 'America/Phoenix' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('timezone=America%2FPhoenix')
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
            timezone: 'America/Los_Angeles',
            limit: 10,
            timeFilter: 'past',
          }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('timezone=America%2FLos_Angeles')
      expect(calledUrl).toContain('limit=10')
      expect(calledUrl).toContain('time_filter=past')
    })
  })
})
