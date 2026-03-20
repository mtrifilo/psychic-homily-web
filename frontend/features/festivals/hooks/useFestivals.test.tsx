import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    FESTIVALS: {
      LIST: '/festivals',
      GET: (idOrSlug: string | number) => `/festivals/${idOrSlug}`,
      ARTISTS: (festivalId: string | number) =>
        `/festivals/${festivalId}/artists`,
      VENUES: (festivalId: string | number) =>
        `/festivals/${festivalId}/venues`,
      ARTIST_FESTIVALS: (artistIdOrSlug: string | number) =>
        `/artists/${artistIdOrSlug}/festivals`,
      SIMILAR: (festivalId: string | number) =>
        `/festivals/${festivalId}/similar`,
      BREAKOUTS: (festivalId: string | number) =>
        `/festivals/${festivalId}/breakouts`,
      ARTIST_TRAJECTORY: (artistIdOrSlug: string | number) =>
        `/artists/${artistIdOrSlug}/festival-trajectory`,
      SERIES_COMPARE: (seriesSlug: string) =>
        `/festivals/series/${seriesSlug}/compare`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    festivals: {
      list: (filters?: Record<string, unknown>) => ['festivals', 'list', filters],
      detail: (idOrSlug: string | number) => ['festivals', 'detail', String(idOrSlug)],
      artists: (idOrSlug: string | number, dayDate?: string) =>
        ['festivals', 'artists', String(idOrSlug), dayDate],
      venues: (idOrSlug: string | number) =>
        ['festivals', 'venues', String(idOrSlug)],
      artistFestivals: (artistIdOrSlug: string | number) =>
        ['festivals', 'artist', String(artistIdOrSlug)],
      similar: (idOrSlug: string | number) =>
        ['festivals', 'similar', String(idOrSlug)],
      breakouts: (idOrSlug: string | number) =>
        ['festivals', 'breakouts', String(idOrSlug)],
      artistTrajectory: (artistIdOrSlug: string | number) =>
        ['festivals', 'trajectory', String(artistIdOrSlug)],
      seriesCompare: (seriesSlug: string, years: number[]) =>
        ['festivals', 'series', seriesSlug, years.join(',')],
    },
  },
}))

// Import hooks after mocks are set up
import {
  useFestivals,
  useFestival,
  useFestivalArtists,
  useFestivalLineup,
  useFestivalVenues,
  useArtistFestivals,
  useSimilarFestivals,
  useFestivalBreakouts,
  useArtistFestivalTrajectory,
  useSeriesComparison,
} from './useFestivals'

describe('Festival Hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  // ──────────────────────────────────────────────
  // useFestivals
  // ──────────────────────────────────────────────

  describe('useFestivals', () => {
    it('fetches festivals list without filters', async () => {
      const mockResponse = {
        festivals: [
          { id: 1, name: 'Festival A', slug: 'festival-a' },
          { id: 2, name: 'Festival B', slug: 'festival-b' },
        ],
        total: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useFestivals(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/festivals', {
        method: 'GET',
      })
      expect(result.current.data?.festivals).toHaveLength(2)
    })

    it('includes status filter in query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ festivals: [], total: 0 })

      const { result } = renderHook(
        () => useFestivals({ status: 'upcoming' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/festivals?status=upcoming',
        { method: 'GET' }
      )
    })

    it('includes city and state filters', async () => {
      mockApiRequest.mockResolvedValueOnce({ festivals: [], total: 0 })

      const { result } = renderHook(
        () => useFestivals({ city: 'Phoenix', state: 'AZ' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('city=Phoenix')
      expect(calledUrl).toContain('state=AZ')
    })

    it('includes year filter', async () => {
      mockApiRequest.mockResolvedValueOnce({ festivals: [], total: 0 })

      const { result } = renderHook(
        () => useFestivals({ year: 2026 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/festivals?year=2026', {
        method: 'GET',
      })
    })

    it('includes series_slug filter', async () => {
      mockApiRequest.mockResolvedValueOnce({ festivals: [], total: 0 })

      const { result } = renderHook(
        () => useFestivals({ seriesSlug: 'coachella' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/festivals?series_slug=coachella',
        { method: 'GET' }
      )
    })

    it('combines multiple filters', async () => {
      mockApiRequest.mockResolvedValueOnce({ festivals: [], total: 0 })

      const { result } = renderHook(
        () =>
          useFestivals({
            status: 'upcoming',
            city: 'Phoenix',
            state: 'AZ',
            year: 2026,
          }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('status=upcoming')
      expect(calledUrl).toContain('city=Phoenix')
      expect(calledUrl).toContain('state=AZ')
      expect(calledUrl).toContain('year=2026')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useFestivals(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  // ──────────────────────────────────────────────
  // useFestival
  // ──────────────────────────────────────────────

  describe('useFestival', () => {
    it('fetches a single festival by slug', async () => {
      const mockFestival = {
        id: 1,
        name: 'Desert Daze',
        slug: 'desert-daze',
        start_date: '2026-10-10',
        end_date: '2026-10-12',
      }
      mockApiRequest.mockResolvedValueOnce(mockFestival)

      const { result } = renderHook(
        () => useFestival({ idOrSlug: 'desert-daze' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/festivals/desert-daze', {
        method: 'GET',
      })
      expect(result.current.data?.name).toBe('Desert Daze')
    })

    it('fetches a single festival by numeric ID', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 5, name: 'Fest' })

      const { result } = renderHook(
        () => useFestival({ idOrSlug: 5 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/festivals/5', {
        method: 'GET',
      })
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () => useFestival({ idOrSlug: 'desert-daze', enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when idOrSlug is empty string', async () => {
      const { result } = renderHook(
        () => useFestival({ idOrSlug: '' }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when numeric ID is 0 or negative', async () => {
      const { result: result0 } = renderHook(
        () => useFestival({ idOrSlug: 0 }),
        { wrapper: createWrapper() }
      )

      const { result: resultNeg } = renderHook(
        () => useFestival({ idOrSlug: -1 }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result0.current.fetchStatus).toBe('idle')
      expect(resultNeg.current.fetchStatus).toBe('idle')
    })

    it('handles not found error', async () => {
      const error = new Error('Festival not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () => useFestival({ idOrSlug: 'nonexistent' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).message).toBe(
        'Festival not found'
      )
    })
  })

  // ──────────────────────────────────────────────
  // useFestivalArtists
  // ──────────────────────────────────────────────

  describe('useFestivalArtists', () => {
    it('fetches the lineup for a festival', async () => {
      const mockResponse = {
        artists: [
          { id: 1, name: 'Artist A', billing_tier: 'headliner' },
          { id: 2, name: 'Artist B', billing_tier: 'undercard' },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useFestivalArtists({ festivalIdOrSlug: 'desert-daze' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/festivals/desert-daze/artists',
        { method: 'GET' }
      )
      expect(result.current.data?.artists).toHaveLength(2)
    })

    it('includes day_date filter', async () => {
      mockApiRequest.mockResolvedValueOnce({ artists: [] })

      const { result } = renderHook(
        () =>
          useFestivalArtists({
            festivalIdOrSlug: 'desert-daze',
            dayDate: '2026-10-11',
          }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/festivals/desert-daze/artists?day_date=2026-10-11',
        { method: 'GET' }
      )
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () =>
          useFestivalArtists({
            festivalIdOrSlug: 'desert-daze',
            enabled: false,
          }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when festivalIdOrSlug is empty string', async () => {
      const { result } = renderHook(
        () => useFestivalArtists({ festivalIdOrSlug: '' }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when numeric ID is 0 or negative', async () => {
      const { result } = renderHook(
        () => useFestivalArtists({ festivalIdOrSlug: 0 }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () => useFestivalArtists({ festivalIdOrSlug: 'desert-daze' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  // ──────────────────────────────────────────────
  // useFestivalLineup (alias)
  // ──────────────────────────────────────────────

  describe('useFestivalLineup', () => {
    it('delegates to useFestivalArtists', async () => {
      mockApiRequest.mockResolvedValueOnce({ artists: [] })

      const { result } = renderHook(
        () => useFestivalLineup({ festivalId: 'desert-daze' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/festivals/desert-daze/artists',
        { method: 'GET' }
      )
    })

    it('passes dayDate through', async () => {
      mockApiRequest.mockResolvedValueOnce({ artists: [] })

      const { result } = renderHook(
        () =>
          useFestivalLineup({
            festivalId: 'desert-daze',
            dayDate: '2026-10-10',
          }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/festivals/desert-daze/artists?day_date=2026-10-10',
        { method: 'GET' }
      )
    })
  })

  // ──────────────────────────────────────────────
  // useFestivalVenues
  // ──────────────────────────────────────────────

  describe('useFestivalVenues', () => {
    it('fetches venues for a festival', async () => {
      const mockResponse = {
        venues: [
          { id: 1, name: 'Main Stage' },
          { id: 2, name: 'Side Stage' },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useFestivalVenues({ festivalIdOrSlug: 'desert-daze' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/festivals/desert-daze/venues',
        { method: 'GET' }
      )
      expect(result.current.data?.venues).toHaveLength(2)
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () =>
          useFestivalVenues({
            festivalIdOrSlug: 'desert-daze',
            enabled: false,
          }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when festivalIdOrSlug is empty string', async () => {
      const { result } = renderHook(
        () => useFestivalVenues({ festivalIdOrSlug: '' }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () => useFestivalVenues({ festivalIdOrSlug: 1 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  // ──────────────────────────────────────────────
  // useArtistFestivals
  // ──────────────────────────────────────────────

  describe('useArtistFestivals', () => {
    it('fetches festivals for an artist by slug', async () => {
      const mockResponse = {
        festivals: [
          { id: 1, name: 'Fest A', billing_tier: 'headliner' },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useArtistFestivals({ artistIdOrSlug: 'the-smile' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/artists/the-smile/festivals',
        { method: 'GET' }
      )
    })

    it('fetches festivals for an artist by numeric ID', async () => {
      mockApiRequest.mockResolvedValueOnce({ festivals: [] })

      const { result } = renderHook(
        () => useArtistFestivals({ artistIdOrSlug: 42 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/artists/42/festivals', {
        method: 'GET',
      })
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () => useArtistFestivals({ artistIdOrSlug: 'the-smile', enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when artistIdOrSlug is empty string', async () => {
      const { result } = renderHook(
        () => useArtistFestivals({ artistIdOrSlug: '' }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when numeric ID is 0', async () => {
      const { result } = renderHook(
        () => useArtistFestivals({ artistIdOrSlug: 0 }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () => useArtistFestivals({ artistIdOrSlug: 'the-smile' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  // ──────────────────────────────────────────────
  // Festival Intelligence hooks
  // ──────────────────────────────────────────────

  describe('useSimilarFestivals', () => {
    it('fetches similar festivals with default limit', async () => {
      const mockResponse = {
        festivals: [
          { id: 2, name: 'Similar Fest', overlap_score: 0.75 },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useSimilarFestivals({ festivalIdOrSlug: 'desert-daze' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('/festivals/desert-daze/similar')
      expect(calledUrl).toContain('limit=10')
    })

    it('fetches similar festivals with custom limit', async () => {
      mockApiRequest.mockResolvedValueOnce({ festivals: [] })

      const { result } = renderHook(
        () =>
          useSimilarFestivals({
            festivalIdOrSlug: 'desert-daze',
            limit: 5,
          }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('limit=5')
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () =>
          useSimilarFestivals({
            festivalIdOrSlug: 'desert-daze',
            enabled: false,
          }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when festivalIdOrSlug is empty', async () => {
      const { result } = renderHook(
        () => useSimilarFestivals({ festivalIdOrSlug: '' }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () => useSimilarFestivals({ festivalIdOrSlug: 1 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useFestivalBreakouts', () => {
    it('fetches breakout artists for a festival', async () => {
      const mockResponse = {
        breakouts: [
          { artist_id: 1, artist_name: 'Rising Star', tier_change: 2 },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useFestivalBreakouts({ festivalIdOrSlug: 'desert-daze' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/festivals/desert-daze/breakouts',
        { method: 'GET' }
      )
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () =>
          useFestivalBreakouts({
            festivalIdOrSlug: 'desert-daze',
            enabled: false,
          }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when festivalIdOrSlug is empty', async () => {
      const { result } = renderHook(
        () => useFestivalBreakouts({ festivalIdOrSlug: '' }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () => useFestivalBreakouts({ festivalIdOrSlug: 1 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useArtistFestivalTrajectory', () => {
    it('fetches trajectory for an artist', async () => {
      const mockResponse = {
        artist_id: 1,
        trajectory: [
          { festival: 'Fest A', year: 2024, tier: 'undercard' },
          { festival: 'Fest A', year: 2025, tier: 'mid_card' },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useArtistFestivalTrajectory({ artistIdOrSlug: 'the-smile' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/artists/the-smile/festival-trajectory',
        { method: 'GET' }
      )
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () =>
          useArtistFestivalTrajectory({
            artistIdOrSlug: 'the-smile',
            enabled: false,
          }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when artistIdOrSlug is empty', async () => {
      const { result } = renderHook(
        () => useArtistFestivalTrajectory({ artistIdOrSlug: '' }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when numeric ID is 0', async () => {
      const { result } = renderHook(
        () => useArtistFestivalTrajectory({ artistIdOrSlug: 0 }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () => useArtistFestivalTrajectory({ artistIdOrSlug: 42 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useSeriesComparison', () => {
    it('fetches year-over-year comparison', async () => {
      const mockResponse = {
        series_slug: 'coachella',
        comparisons: [
          { year: 2025, artist_count: 100 },
          { year: 2026, artist_count: 110 },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () =>
          useSeriesComparison({
            seriesSlug: 'coachella',
            years: [2025, 2026],
          }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('/festivals/series/coachella/compare')
      expect(calledUrl).toContain('years=2025%2C2026')
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () =>
          useSeriesComparison({
            seriesSlug: 'coachella',
            years: [2025, 2026],
            enabled: false,
          }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when seriesSlug is empty', async () => {
      const { result } = renderHook(
        () =>
          useSeriesComparison({
            seriesSlug: '',
            years: [2025, 2026],
          }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when fewer than 2 years provided', async () => {
      const { result } = renderHook(
        () =>
          useSeriesComparison({
            seriesSlug: 'coachella',
            years: [2025],
          }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when years array is empty', async () => {
      const { result } = renderHook(
        () =>
          useSeriesComparison({
            seriesSlug: 'coachella',
            years: [],
          }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () =>
          useSeriesComparison({
            seriesSlug: 'coachella',
            years: [2025, 2026],
          }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })
})
