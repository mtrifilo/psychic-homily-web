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
vi.mock('@/features/venues/api', () => ({
  venueEndpoints: {
    LIST: '/venues',
    CITIES: '/venues/cities',
    GET: (venueId: string | number) => `/venues/${venueId}`,
    SHOWS: (venueId: string | number) => `/venues/${venueId}/shows`,
  },
  venueQueryKeys: {
    list: (filters?: Record<string, unknown>) => ['venues', 'list', filters],
    detail: (id: string) => ['venues', 'detail', id],
    shows: (venueId: string | number) => ['venues', 'shows', String(venueId)],
    cities: ['venues', 'cities'],
  },
}))

// Import hooks after mocks are set up
import { useVenues, useVenue, useVenueShows, useVenueCities } from './useVenues'


describe('useVenues', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('useVenues (list)', () => {
    it('fetches venues with default options', async () => {
      const mockResponse = {
        venues: [
          { id: 1, name: 'Venue 1', city: 'Phoenix', state: 'AZ' },
          { id: 2, name: 'Venue 2', city: 'Tempe', state: 'AZ' },
        ],
        total: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useVenues(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      // Default limit is 50 (offset=0 is not included since it's falsy)
      expect(mockApiRequest).toHaveBeenCalledWith(
        '/venues?limit=50',
        { method: 'GET' }
      )
    })

    it('filters by state', async () => {
      mockApiRequest.mockResolvedValueOnce({ venues: [], total: 0 })

      const { result } = renderHook(() => useVenues({ state: 'AZ' }), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).toContain('state=AZ')
    })

    it('filters by city', async () => {
      mockApiRequest.mockResolvedValueOnce({ venues: [], total: 0 })

      const { result } = renderHook(() => useVenues({ city: 'Phoenix' }), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).toContain('city=Phoenix')
    })

    it('supports custom limit and offset for pagination', async () => {
      mockApiRequest.mockResolvedValueOnce({ venues: [], total: 0 })

      const { result } = renderHook(
        () => useVenues({ limit: 25, offset: 50 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).toContain('limit=25')
      expect(mockApiRequest.mock.calls[0][0]).toContain('offset=50')
    })

    it('combines multiple filters', async () => {
      mockApiRequest.mockResolvedValueOnce({ venues: [], total: 0 })

      const { result } = renderHook(
        () => useVenues({ state: 'AZ', city: 'Phoenix', limit: 10, offset: 0 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('state=AZ')
      expect(calledUrl).toContain('city=Phoenix')
      expect(calledUrl).toContain('limit=10')
    })

  })

  describe('useVenue (detail)', () => {
    it('fetches a single venue by ID', async () => {
      const mockVenue = {
        id: 1,
        name: 'The Rebel Lounge',
        city: 'Phoenix',
        state: 'AZ',
        address: '2303 E Indian School Rd',
        verified: true,
      }
      mockApiRequest.mockResolvedValueOnce(mockVenue)

      const { result } = renderHook(() => useVenue({ venueId: 1 }), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/venues/1', {
        method: 'GET',
      })
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () => useVenue({ venueId: 1, enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when venueId is 0 or negative', async () => {
      const { result: result0 } = renderHook(
        () => useVenue({ venueId: 0 }),
        { wrapper: createWrapper() }
      )

      const { result: resultNeg } = renderHook(
        () => useVenue({ venueId: -1 }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result0.current.fetchStatus).toBe('idle')
      expect(resultNeg.current.fetchStatus).toBe('idle')
    })

    it('handles venue not found error', async () => {
      const error = new Error('Venue not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useVenue({ venueId: 999 }), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Venue not found')
    })

  })

  describe('useVenueShows', () => {
    it('fetches shows for a venue with default options', async () => {
      const mockResponse = {
        shows: [
          { id: 1, title: 'Show 1' },
          { id: 2, title: 'Show 2' },
        ],
        total: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useVenueShows({ venueId: 1 }), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      // Default time_filter is 'upcoming', default limit is 20
      expect(mockApiRequest).toHaveBeenCalledWith(
        '/venues/1/shows?limit=20&time_filter=upcoming',
        { method: 'GET' }
      )
    })

    it('includes timezone in query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () => useVenueShows({ venueId: 1, timezone: 'America/Phoenix' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('timezone=America%2FPhoenix')
    })

    it('supports upcoming time filter', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () => useVenueShows({ venueId: 1, timeFilter: 'upcoming' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).toContain('time_filter=upcoming')
    })

    it('supports past time filter', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () => useVenueShows({ venueId: 1, timeFilter: 'past' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).toContain('time_filter=past')
    })

    it('supports all time filter', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () => useVenueShows({ venueId: 1, timeFilter: 'all' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).toContain('time_filter=all')
    })

    it('supports custom limit', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () => useVenueShows({ venueId: 1, limit: 50 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).toContain('limit=50')
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () => useVenueShows({ venueId: 1, enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when venueId is invalid', async () => {
      const { result } = renderHook(
        () => useVenueShows({ venueId: 0 }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

  })

  describe('useVenueCities', () => {
    it('fetches list of cities with venue counts', async () => {
      const mockResponse = {
        cities: [
          { city: 'Phoenix', state: 'AZ', venue_count: 25 },
          { city: 'Tempe', state: 'AZ', venue_count: 12 },
          { city: 'Mesa', state: 'AZ', venue_count: 8 },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useVenueCities(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/venues/cities', {
        method: 'GET',
      })
    })

    it('returns empty list when no cities', async () => {
      mockApiRequest.mockResolvedValueOnce({ cities: [] })

      const { result } = renderHook(() => useVenueCities(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
    })
  })
})
