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
    SCENES: {
      LIST: '/scenes',
      DETAIL: (slug: string) => `/scenes/${slug}`,
      ARTISTS: (slug: string) => `/scenes/${slug}/artists`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    scenes: {
      list: ['scenes', 'list'],
      detail: (slug: string) => ['scenes', 'detail', slug],
      artists: (slug: string, period?: number) =>
        ['scenes', 'artists', slug, period],
    },
  },
}))

// Import hooks after mocks are set up
import { useScenes, useSceneDetail, useSceneArtists } from './useScenes'

describe('Scene Hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  // ──────────────────────────────────────────────
  // useScenes
  // ──────────────────────────────────────────────

  describe('useScenes', () => {
    it('fetches the list of scenes', async () => {
      const mockResponse = {
        scenes: [
          { slug: 'phoenix-az', city: 'Phoenix', state: 'AZ', artist_count: 50 },
          { slug: 'mesa-az', city: 'Mesa', state: 'AZ', artist_count: 20 },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useScenes(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/scenes', {
        method: 'GET',
      })
      expect(result.current.data?.scenes).toHaveLength(2)
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useScenes(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })

    it('returns empty scenes list', async () => {
      mockApiRequest.mockResolvedValueOnce({ scenes: [] })

      const { result } = renderHook(() => useScenes(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.scenes).toHaveLength(0)
    })
  })

  // ──────────────────────────────────────────────
  // useSceneDetail
  // ──────────────────────────────────────────────

  describe('useSceneDetail', () => {
    it('fetches scene detail by slug', async () => {
      const mockDetail = {
        slug: 'phoenix-az',
        city: 'Phoenix',
        state: 'AZ',
        artist_count: 50,
        venue_count: 12,
        upcoming_show_count: 30,
      }
      mockApiRequest.mockResolvedValueOnce(mockDetail)

      const { result } = renderHook(() => useSceneDetail('phoenix-az'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/scenes/phoenix-az', {
        method: 'GET',
      })
      expect(result.current.data?.city).toBe('Phoenix')
    })

    it('does not fetch when slug is empty', async () => {
      const { result } = renderHook(() => useSceneDetail(''), {
        wrapper: createWrapper(),
      })

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles scene not found error', async () => {
      const error = new Error('Scene not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useSceneDetail('nonexistent'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).message).toBe('Scene not found')
    })
  })

  // ──────────────────────────────────────────────
  // useSceneArtists
  // ──────────────────────────────────────────────

  describe('useSceneArtists', () => {
    it('fetches scene artists with default options', async () => {
      const mockResponse = {
        artists: [
          { id: 1, name: 'Artist A', show_count: 5 },
          { id: 2, name: 'Artist B', show_count: 3 },
        ],
        total: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useSceneArtists({ slug: 'phoenix-az' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      // Default period=90, limit=20, offset=0
      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('/scenes/phoenix-az/artists')
      expect(calledUrl).toContain('period=90')
      expect(calledUrl).toContain('limit=20')
      expect(result.current.data?.artists).toHaveLength(2)
    })

    it('includes custom period in query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ artists: [], total: 0 })

      const { result } = renderHook(
        () => useSceneArtists({ slug: 'phoenix-az', period: 180 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('period=180')
    })

    it('includes custom limit in query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ artists: [], total: 0 })

      const { result } = renderHook(
        () => useSceneArtists({ slug: 'phoenix-az', limit: 50 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('limit=50')
    })

    it('includes offset in query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ artists: [], total: 0 })

      const { result } = renderHook(
        () => useSceneArtists({ slug: 'phoenix-az', offset: 20 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('offset=20')
    })

    it('combines multiple query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ artists: [], total: 0 })

      const { result } = renderHook(
        () =>
          useSceneArtists({
            slug: 'phoenix-az',
            period: 365,
            limit: 10,
            offset: 30,
          }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('period=365')
      expect(calledUrl).toContain('limit=10')
      expect(calledUrl).toContain('offset=30')
    })

    it('does not fetch when slug is empty', async () => {
      const { result } = renderHook(
        () => useSceneArtists({ slug: '' }),
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
        () => useSceneArtists({ slug: 'phoenix-az' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })
})
