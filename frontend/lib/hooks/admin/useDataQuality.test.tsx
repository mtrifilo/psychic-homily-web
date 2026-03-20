import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('../../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      DATA_QUALITY: {
        SUMMARY: '/admin/data-quality',
        CATEGORY: (category: string) => `/admin/data-quality/${category}`,
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('../../queryClient', () => ({
  queryKeys: {
    admin: {
      dataQuality: {
        summary: ['admin', 'dataQuality', 'summary'],
        category: (category: string, limit: number, offset: number) => [
          'admin',
          'dataQuality',
          'category',
          category,
          { limit, offset },
        ],
      },
    },
  },
}))

// Import hooks after mocks are set up
import { useDataQualitySummary, useDataQualityCategory } from './useDataQuality'

describe('useDataQuality', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('useDataQualitySummary', () => {
    it('fetches data quality summary successfully', async () => {
      const mockResponse = {
        categories: [
          {
            key: 'artists_no_shows',
            label: 'Artists with no shows',
            entity_type: 'artist',
            count: 15,
            description: 'Artists that have no associated shows',
          },
          {
            key: 'venues_no_shows',
            label: 'Venues with no shows',
            entity_type: 'venue',
            count: 8,
            description: 'Venues that have no associated shows',
          },
          {
            key: 'shows_no_artists',
            label: 'Shows with no artists',
            entity_type: 'show',
            count: 3,
            description: 'Shows that have no associated artists',
          },
        ],
        total_items: 26,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useDataQualitySummary(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/admin/data-quality', {
        method: 'GET',
      })
      expect(result.current.data?.categories).toHaveLength(3)
      expect(result.current.data?.total_items).toBe(26)
    })

    it('handles summary with zero counts', async () => {
      const mockResponse = {
        categories: [
          {
            key: 'artists_no_shows',
            label: 'Artists with no shows',
            entity_type: 'artist',
            count: 0,
            description: 'Artists that have no associated shows',
          },
        ],
        total_items: 0,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useDataQualitySummary(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.categories[0].count).toBe(0)
      expect(result.current.data?.total_items).toBe(0)
    })

    it('handles empty categories list', async () => {
      mockApiRequest.mockResolvedValueOnce({
        categories: [],
        total_items: 0,
      })

      const { result } = renderHook(() => useDataQualitySummary(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.categories).toHaveLength(0)
    })

    it('handles API error', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useDataQualitySummary(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })

    it('handles authentication error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useDataQualitySummary(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Forbidden')
    })
  })

  describe('useDataQualityCategory', () => {
    it('fetches items for a specific category', async () => {
      const mockResponse = {
        items: [
          {
            entity_type: 'artist',
            entity_id: 1,
            name: 'Unknown Artist',
            slug: 'unknown-artist',
            reason: 'No associated shows',
            show_count: 0,
          },
          {
            entity_type: 'artist',
            entity_id: 2,
            name: 'Orphan Band',
            slug: 'orphan-band',
            reason: 'No associated shows',
            show_count: 0,
          },
        ],
        total: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useDataQualityCategory('artists_no_shows'),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/data-quality/artists_no_shows?limit=50&offset=0',
        { method: 'GET' }
      )
      expect(result.current.data?.items).toHaveLength(2)
      expect(result.current.data?.total).toBe(2)
    })

    it('supports custom limit and offset', async () => {
      mockApiRequest.mockResolvedValueOnce({ items: [], total: 0 })

      const { result } = renderHook(
        () => useDataQualityCategory('venues_no_shows', 10, 20),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/data-quality/venues_no_shows?limit=10&offset=20',
        { method: 'GET' }
      )
    })

    it('handles empty items list', async () => {
      mockApiRequest.mockResolvedValueOnce({ items: [], total: 0 })

      const { result } = renderHook(
        () => useDataQualityCategory('artists_no_shows'),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.items).toHaveLength(0)
      expect(result.current.data?.total).toBe(0)
    })

    it('is disabled when category is empty string', async () => {
      const { result } = renderHook(
        () => useDataQualityCategory(''),
        { wrapper: createWrapper() }
      )

      // Should not fetch because category is empty
      expect(result.current.isFetching).toBe(false)
      expect(mockApiRequest).not.toHaveBeenCalled()
    })

    it('respects enabled option', async () => {
      const { result } = renderHook(
        () =>
          useDataQualityCategory('artists_no_shows', 50, 0, {
            enabled: false,
          }),
        { wrapper: createWrapper() }
      )

      // Should not fetch because enabled is false
      expect(result.current.isFetching).toBe(false)
      expect(mockApiRequest).not.toHaveBeenCalled()
    })

    it('is enabled by default when category is provided', async () => {
      mockApiRequest.mockResolvedValueOnce({ items: [], total: 0 })

      const { result } = renderHook(
        () => useDataQualityCategory('shows_no_artists'),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalled()
    })

    it('handles items with various show counts', async () => {
      const mockResponse = {
        items: [
          {
            entity_type: 'artist',
            entity_id: 10,
            name: 'Popular Artist',
            slug: 'popular-artist',
            reason: 'Missing genre tags',
            show_count: 25,
          },
          {
            entity_type: 'artist',
            entity_id: 11,
            name: 'New Artist',
            slug: 'new-artist',
            reason: 'Missing genre tags',
            show_count: 1,
          },
        ],
        total: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useDataQualityCategory('missing_genre_tags'),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.items[0].show_count).toBe(25)
      expect(result.current.data?.items[1].show_count).toBe(1)
    })

    it('handles API error', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () => useDataQualityCategory('artists_no_shows'),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
    })

    it('handles authentication error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () => useDataQualityCategory('artists_no_shows'),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Forbidden')
    })

    it('handles 404 for unknown category', async () => {
      const error = new Error('Category not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () => useDataQualityCategory('nonexistent_category'),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe(
        'Category not found'
      )
    })
  })
})
