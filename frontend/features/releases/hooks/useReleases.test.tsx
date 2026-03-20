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
    RELEASES: {
      LIST: '/releases',
      GET: (idOrSlug: string | number) => `/releases/${idOrSlug}`,
      ARTIST_RELEASES: (artistIdOrSlug: string | number) =>
        `/artists/${artistIdOrSlug}/releases`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    releases: {
      list: (filters?: Record<string, unknown>) => ['releases', 'list', filters],
      detail: (idOrSlug: string | number) => ['releases', 'detail', String(idOrSlug)],
      artistReleases: (artistIdOrSlug: string | number) =>
        ['releases', 'artist', String(artistIdOrSlug)],
    },
  },
}))

// Import hooks after mocks are set up
import { useReleases, useRelease, useArtistReleases } from './useReleases'

describe('Release Hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  // ──────────────────────────────────────────────
  // useReleases
  // ──────────────────────────────────────────────

  describe('useReleases', () => {
    it('fetches releases list without filters', async () => {
      const mockResponse = {
        releases: [
          { id: 1, title: 'Album A', slug: 'album-a' },
          { id: 2, title: 'Album B', slug: 'album-b' },
        ],
        total: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useReleases(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/releases', {
        method: 'GET',
      })
      expect(result.current.data?.releases).toHaveLength(2)
    })

    it('includes release_type filter', async () => {
      mockApiRequest.mockResolvedValueOnce({ releases: [], total: 0 })

      const { result } = renderHook(
        () => useReleases({ releaseType: 'album' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/releases?release_type=album',
        { method: 'GET' }
      )
    })

    it('includes year filter', async () => {
      mockApiRequest.mockResolvedValueOnce({ releases: [], total: 0 })

      const { result } = renderHook(
        () => useReleases({ year: 2025 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/releases?year=2025', {
        method: 'GET',
      })
    })

    it('includes artist_id filter', async () => {
      mockApiRequest.mockResolvedValueOnce({ releases: [], total: 0 })

      const { result } = renderHook(
        () => useReleases({ artistId: 42 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/releases?artist_id=42', {
        method: 'GET',
      })
    })

    it('includes string artist_id filter', async () => {
      mockApiRequest.mockResolvedValueOnce({ releases: [], total: 0 })

      const { result } = renderHook(
        () => useReleases({ artistId: 'the-smile' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/releases?artist_id=the-smile',
        { method: 'GET' }
      )
    })

    it('combines multiple filters', async () => {
      mockApiRequest.mockResolvedValueOnce({ releases: [], total: 0 })

      const { result } = renderHook(
        () => useReleases({ releaseType: 'ep', year: 2026, artistId: 10 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('release_type=ep')
      expect(calledUrl).toContain('year=2026')
      expect(calledUrl).toContain('artist_id=10')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useReleases(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })

    it('returns empty releases list', async () => {
      mockApiRequest.mockResolvedValueOnce({ releases: [], total: 0 })

      const { result } = renderHook(() => useReleases(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.releases).toHaveLength(0)
    })
  })

  // ──────────────────────────────────────────────
  // useRelease
  // ──────────────────────────────────────────────

  describe('useRelease', () => {
    it('fetches a single release by slug', async () => {
      const mockRelease = {
        id: 1,
        title: 'Cutouts',
        slug: 'cutouts',
        release_type: 'album',
        release_date: '2024-10-04',
      }
      mockApiRequest.mockResolvedValueOnce(mockRelease)

      const { result } = renderHook(
        () => useRelease({ idOrSlug: 'cutouts' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/releases/cutouts', {
        method: 'GET',
      })
      expect(result.current.data?.title).toBe('Cutouts')
    })

    it('fetches a single release by numeric ID', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 5, title: 'Release' })

      const { result } = renderHook(
        () => useRelease({ idOrSlug: 5 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/releases/5', {
        method: 'GET',
      })
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () => useRelease({ idOrSlug: 'cutouts', enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when idOrSlug is empty string', async () => {
      const { result } = renderHook(
        () => useRelease({ idOrSlug: '' }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when numeric ID is 0 or negative', async () => {
      const { result: result0 } = renderHook(
        () => useRelease({ idOrSlug: 0 }),
        { wrapper: createWrapper() }
      )
      const { result: resultNeg } = renderHook(
        () => useRelease({ idOrSlug: -1 }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result0.current.fetchStatus).toBe('idle')
      expect(resultNeg.current.fetchStatus).toBe('idle')
    })

    it('handles release not found error', async () => {
      const error = new Error('Release not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () => useRelease({ idOrSlug: 'nonexistent' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).message).toBe(
        'Release not found'
      )
    })
  })

  // ──────────────────────────────────────────────
  // useArtistReleases
  // ──────────────────────────────────────────────

  describe('useArtistReleases', () => {
    it('fetches releases for an artist by slug', async () => {
      const mockResponse = {
        releases: [
          { id: 1, title: 'Album A' },
          { id: 2, title: 'Album B' },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useArtistReleases({ artistIdOrSlug: 'the-smile' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/artists/the-smile/releases',
        { method: 'GET' }
      )
      expect(result.current.data?.releases).toHaveLength(2)
    })

    it('fetches releases for an artist by numeric ID', async () => {
      mockApiRequest.mockResolvedValueOnce({ releases: [] })

      const { result } = renderHook(
        () => useArtistReleases({ artistIdOrSlug: 42 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/artists/42/releases', {
        method: 'GET',
      })
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () =>
          useArtistReleases({ artistIdOrSlug: 'the-smile', enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when artistIdOrSlug is empty string', async () => {
      const { result } = renderHook(
        () => useArtistReleases({ artistIdOrSlug: '' }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when numeric ID is 0', async () => {
      const { result } = renderHook(
        () => useArtistReleases({ artistIdOrSlug: 0 }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when numeric ID is negative', async () => {
      const { result } = renderHook(
        () => useArtistReleases({ artistIdOrSlug: -3 }),
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
        () => useArtistReleases({ artistIdOrSlug: 'the-smile' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })
})
