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
    // Mirrors the real builder. This file mocks the api module, so it cannot
    // assert the real key — useVenueShowsCacheKey.test.tsx does that against
    // the genuine one.
    showsPage: (
      venueId: string | number,
      params: { timeFilter: string; limit?: number },
    ) => [
      'venues',
      'shows',
      String(venueId),
      params.timeFilter,
      params.limit ?? null,
    ],
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

    // PSY-1539: the Atlas rail fields cost three extra server-side
    // aggregations, so they are opt-in — the browse page must not ask for them.
    it('does not request the Atlas rail fields by default', async () => {
      mockApiRequest.mockResolvedValueOnce({ venues: [], total: 0 })

      const { result } = renderHook(() => useVenues(), {
        wrapper: createWrapper(),
      })
      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).not.toContain('include_rail')
    })

    it('requests the Atlas rail fields when asked', async () => {
      mockApiRequest.mockResolvedValueOnce({ venues: [], total: 0 })

      const { result } = renderHook(
        () => useVenues({ city: 'Austin', state: 'TX', includeRail: true }),
        { wrapper: createWrapper() },
      )
      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).toContain('include_rail=true')
    })

    // PSY-1574: the browse page's city filter means the LITERAL city, so
    // widening to the metro is opt-in the same way the rail fields are.
    it('does not roll a city filter up to its metro by default', async () => {
      mockApiRequest.mockResolvedValueOnce({ venues: [], total: 0 })

      const { result } = renderHook(
        () => useVenues({ city: 'Phoenix', state: 'AZ' }),
        { wrapper: createWrapper() },
      )
      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).not.toContain('metro_rollup')
    })

    it('rolls the city filter up to the metro when asked', async () => {
      mockApiRequest.mockResolvedValueOnce({ venues: [], total: 0 })

      const { result } = renderHook(
        () => useVenues({ city: 'Phoenix', state: 'AZ', metroRollup: true }),
        { wrapper: createWrapper() },
      )
      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).toContain('metro_rollup=true')
    })

    // The API documents itself as ignoring metro_rollup alongside `cities`;
    // sending it anyway would put a no-op param in the URL and the cache key.
    it('omits the metro rollup when a multi-city filter is used', async () => {
      mockApiRequest.mockResolvedValueOnce({ venues: [], total: 0 })

      const { result } = renderHook(
        () =>
          useVenues({
            cities: [{ city: 'Phoenix', state: 'AZ' }],
            metroRollup: true,
          }),
        { wrapper: createWrapper() },
      )
      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).not.toContain('metro_rollup')
    })

    // The cache key must describe what was SENT. A key that said "metro" over
    // a URL that dropped it would split one byte-identical request across two
    // entries that can never disagree — pure fragmentation, and a second
    // network round trip for a response already in cache.
    it('shares one cache entry when the dropped rollup makes two requests identical', async () => {
      mockApiRequest.mockResolvedValue({ venues: [], total: 0 })
      const wrapper = createWrapper()
      const cities = [{ city: 'Phoenix', state: 'AZ' }]

      const withRollup = renderHook(
        () => useVenues({ cities, metroRollup: true }),
        { wrapper },
      )
      await waitFor(() => expect(withRollup.result.current.isSuccess).toBe(true))

      const withoutRollup = renderHook(() => useVenues({ cities }), { wrapper })
      await waitFor(() =>
        expect(withoutRollup.result.current.isSuccess).toBe(true),
      )

      expect(mockApiRequest).toHaveBeenCalledTimes(1)
    })

    // An unscoped GET /venues is a whole-catalogue page, not a cheap no-op —
    // the Atlas globe view has long stretches with no city resolved yet.
    it('makes no request while disabled', async () => {
      mockApiRequest.mockResolvedValue({ venues: [], total: 0 })

      const { result } = renderHook(() => useVenues({ enabled: false }), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.fetchStatus).toBe('idle'))
      expect(mockApiRequest).not.toHaveBeenCalled()
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
