import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

// Create mock
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('../../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      ANALYTICS: {
        GROWTH: '/admin/analytics/growth',
        ENGAGEMENT: '/admin/analytics/engagement',
        COMMUNITY: '/admin/analytics/community',
        DATA_QUALITY: '/admin/analytics/data-quality',
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('../../queryClient', () => ({
  queryKeys: {
    admin: {
      analytics: {
        growth: (months: number) => ['admin', 'analytics', 'growth', months],
        engagement: (months: number) => ['admin', 'analytics', 'engagement', months],
        community: ['admin', 'analytics', 'community'],
        dataQualityTrends: (months: number) => ['admin', 'analytics', 'data-quality', months],
      },
    },
  },
}))

// Import hooks after mocks
import {
  useGrowthMetrics,
  useEngagementMetrics,
  useCommunityHealth,
  useDataQualityTrends,
} from './useAnalytics'

describe('useAnalytics hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('useGrowthMetrics', () => {
    it('fetches growth metrics with default months', async () => {
      const mockResponse = {
        shows: [{ month: '2026-01', count: 10 }],
        artists: [{ month: '2026-01', count: 5 }],
        venues: [{ month: '2026-01', count: 2 }],
        releases: [{ month: '2026-01', count: 3 }],
        labels: [{ month: '2026-01', count: 1 }],
        users: [{ month: '2026-01', count: 8 }],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useGrowthMetrics(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/analytics/growth?months=6',
        { method: 'GET' }
      )
      expect(result.current.data?.shows).toHaveLength(1)
      expect(result.current.data?.shows[0].count).toBe(10)
    })

    it('fetches growth metrics with custom months', async () => {
      mockApiRequest.mockResolvedValueOnce({
        shows: [],
        artists: [],
        venues: [],
        releases: [],
        labels: [],
        users: [],
      })

      const { result } = renderHook(() => useGrowthMetrics(12), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/analytics/growth?months=12',
        { method: 'GET' }
      )
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useGrowthMetrics(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useEngagementMetrics', () => {
    it('fetches engagement metrics with default months', async () => {
      const mockResponse = {
        bookmarks: [{ month: '2026-01', count: 20 }],
        tags_added: [{ month: '2026-01', count: 15 }],
        tag_votes: [{ month: '2026-01', count: 30 }],
        collection_items: [{ month: '2026-01', count: 10 }],
        requests: [{ month: '2026-01', count: 5 }],
        request_votes: [{ month: '2026-01', count: 12 }],
        revisions: [{ month: '2026-01', count: 8 }],
        follows: [{ month: '2026-01', count: 25 }],
        attendance: [{ month: '2026-01', count: 40 }],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useEngagementMetrics(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/analytics/engagement?months=6',
        { method: 'GET' }
      )
      expect(result.current.data?.bookmarks[0].count).toBe(20)
      expect(result.current.data?.attendance[0].count).toBe(40)
    })

    it('fetches with custom months parameter', async () => {
      mockApiRequest.mockResolvedValueOnce({
        bookmarks: [],
        tags_added: [],
        tag_votes: [],
        collection_items: [],
        requests: [],
        request_votes: [],
        revisions: [],
        follows: [],
        attendance: [],
      })

      const { result } = renderHook(() => useEngagementMetrics(24), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/analytics/engagement?months=24',
        { method: 'GET' }
      )
    })

    it('handles API errors', async () => {
      mockApiRequest.mockRejectedValueOnce(new Error('Forbidden'))

      const { result } = renderHook(() => useEngagementMetrics(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useCommunityHealth', () => {
    it('fetches community health snapshot', async () => {
      const mockResponse = {
        active_contributors_30d: 42,
        contributions_per_week: [
          { week: '2026-W10', count: 15 },
          { week: '2026-W11', count: 20 },
        ],
        request_fulfillment_rate: 0.72,
        new_collections_30d: 8,
        top_contributors: [
          { user_id: 1, username: 'alice', display_name: 'Alice', count: 50 },
          { user_id: 2, username: 'bob', count: 35 },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useCommunityHealth(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/analytics/community',
        { method: 'GET' }
      )
      expect(result.current.data?.active_contributors_30d).toBe(42)
      expect(result.current.data?.request_fulfillment_rate).toBe(0.72)
      expect(result.current.data?.top_contributors).toHaveLength(2)
    })

    it('handles API errors', async () => {
      mockApiRequest.mockRejectedValueOnce(new Error('Server error'))

      const { result } = renderHook(() => useCommunityHealth(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useDataQualityTrends', () => {
    it('fetches data quality trends with default months', async () => {
      const mockResponse = {
        shows_approved: [{ month: '2026-01', count: 100 }],
        shows_rejected: [{ month: '2026-01', count: 15 }],
        pending_review_count: 23,
        artists_without_releases: 45,
        inactive_venues_90d: 12,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useDataQualityTrends(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/analytics/data-quality?months=6',
        { method: 'GET' }
      )
      expect(result.current.data?.pending_review_count).toBe(23)
      expect(result.current.data?.artists_without_releases).toBe(45)
      expect(result.current.data?.inactive_venues_90d).toBe(12)
    })

    it('fetches with custom months parameter', async () => {
      mockApiRequest.mockResolvedValueOnce({
        shows_approved: [],
        shows_rejected: [],
        pending_review_count: 0,
        artists_without_releases: 0,
        inactive_venues_90d: 0,
      })

      const { result } = renderHook(() => useDataQualityTrends(3), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/analytics/data-quality?months=3',
        { method: 'GET' }
      )
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useDataQualityTrends(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })
})
