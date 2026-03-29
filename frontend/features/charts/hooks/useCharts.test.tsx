import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    CHARTS: {
      OVERVIEW: '/charts/overview',
      TRENDING_SHOWS: '/charts/trending-shows',
      POPULAR_ARTISTS: '/charts/popular-artists',
      ACTIVE_VENUES: '/charts/active-venues',
      HOT_RELEASES: '/charts/hot-releases',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    charts: {
      all: ['charts'],
      overview: ['charts', 'overview'],
      trendingShows: (limit?: number) => ['charts', 'trending-shows', limit],
      popularArtists: (limit?: number) => ['charts', 'popular-artists', limit],
      activeVenues: (limit?: number) => ['charts', 'active-venues', limit],
      hotReleases: (limit?: number) => ['charts', 'hot-releases', limit],
    },
  },
}))

import {
  useChartsOverview,
  useTrendingShows,
  usePopularArtists,
  useActiveVenues,
  useHotReleases,
} from './useCharts'

describe('useCharts hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('useChartsOverview', () => {
    it('fetches charts overview data', async () => {
      const mockData = {
        trending_shows: [
          { show_id: 1, title: 'Show A', slug: 'show-a', date: '2026-04-01T20:00:00Z', venue_name: 'Venue A', venue_slug: 'venue-a', city: 'Phoenix', going_count: 10, interested_count: 25, total_attendance: 35 },
        ],
        popular_artists: [
          { artist_id: 1, name: 'Artist A', slug: 'artist-a', image_url: '', follow_count: 50, upcoming_show_count: 3, score: 53 },
        ],
        active_venues: [
          { venue_id: 1, name: 'Venue A', slug: 'venue-a', city: 'Phoenix', state: 'AZ', upcoming_show_count: 12, follow_count: 30, score: 42 },
        ],
        hot_releases: [
          { release_id: 1, title: 'Album A', slug: 'album-a', release_date: '2026-03-01', artist_names: ['Artist A'], bookmark_count: 15 },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockData)

      const { result } = renderHook(() => useChartsOverview(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/charts/overview', { method: 'GET' })
      expect(result.current.data?.trending_shows).toHaveLength(1)
      expect(result.current.data?.popular_artists).toHaveLength(1)
      expect(result.current.data?.active_venues).toHaveLength(1)
      expect(result.current.data?.hot_releases).toHaveLength(1)
    })

  })

  describe('useTrendingShows', () => {
    it('fetches trending shows without limit', async () => {
      const mockData = {
        shows: [
          { show_id: 1, title: 'Show A', slug: 'show-a', date: '2026-04-01T20:00:00Z', venue_name: 'Venue A', venue_slug: 'venue-a', city: 'Phoenix', going_count: 10, interested_count: 25, total_attendance: 35 },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockData)

      const { result } = renderHook(() => useTrendingShows(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/charts/trending-shows', { method: 'GET' })
      expect(result.current.data?.shows).toHaveLength(1)
    })

    it('fetches trending shows with limit', async () => {
      const mockData = { shows: [] }
      mockApiRequest.mockResolvedValueOnce(mockData)

      const { result } = renderHook(() => useTrendingShows(10), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/charts/trending-shows?limit=10', { method: 'GET' })
    })
  })

  describe('usePopularArtists', () => {
    it('fetches popular artists', async () => {
      const mockData = {
        artists: [
          { artist_id: 1, name: 'Artist A', slug: 'artist-a', image_url: '', follow_count: 50, upcoming_show_count: 3, score: 53 },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockData)

      const { result } = renderHook(() => usePopularArtists(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/charts/popular-artists', { method: 'GET' })
      expect(result.current.data?.artists).toHaveLength(1)
      expect(result.current.data?.artists[0].name).toBe('Artist A')
    })

    it('passes limit param', async () => {
      mockApiRequest.mockResolvedValueOnce({ artists: [] })

      const { result } = renderHook(() => usePopularArtists(5), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/charts/popular-artists?limit=5', { method: 'GET' })
    })
  })

  describe('useActiveVenues', () => {
    it('fetches active venues', async () => {
      const mockData = {
        venues: [
          { venue_id: 1, name: 'Venue A', slug: 'venue-a', city: 'Phoenix', state: 'AZ', upcoming_show_count: 12, follow_count: 30, score: 42 },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockData)

      const { result } = renderHook(() => useActiveVenues(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/charts/active-venues', { method: 'GET' })
      expect(result.current.data?.venues).toHaveLength(1)
      expect(result.current.data?.venues[0].name).toBe('Venue A')
    })

    it('passes limit param', async () => {
      mockApiRequest.mockResolvedValueOnce({ venues: [] })

      const { result } = renderHook(() => useActiveVenues(15), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/charts/active-venues?limit=15', { method: 'GET' })
    })
  })

  describe('useHotReleases', () => {
    it('fetches hot releases', async () => {
      const mockData = {
        releases: [
          { release_id: 1, title: 'Album A', slug: 'album-a', release_date: '2026-03-01', artist_names: ['Artist A'], bookmark_count: 15 },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockData)

      const { result } = renderHook(() => useHotReleases(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/charts/hot-releases', { method: 'GET' })
      expect(result.current.data?.releases).toHaveLength(1)
      expect(result.current.data?.releases[0].title).toBe('Album A')
    })

    it('passes limit param', async () => {
      mockApiRequest.mockResolvedValueOnce({ releases: [] })

      const { result } = renderHook(() => useHotReleases(25), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/charts/hot-releases?limit=25', { method: 'GET' })
    })
  })
})
