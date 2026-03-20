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
    LABELS: {
      LIST: '/labels',
      GET: (idOrSlug: string | number) => `/labels/${idOrSlug}`,
      ARTISTS: (idOrSlug: string | number) => `/labels/${idOrSlug}/artists`,
      RELEASES: (idOrSlug: string | number) => `/labels/${idOrSlug}/releases`,
    },
    ARTISTS: {
      LABELS: (artistIdOrSlug: string | number) =>
        `/artists/${artistIdOrSlug}/labels`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    labels: {
      list: (filters?: Record<string, unknown>) => ['labels', 'list', filters],
      detail: (idOrSlug: string | number) => ['labels', 'detail', String(idOrSlug)],
      roster: (idOrSlug: string | number) => ['labels', 'roster', String(idOrSlug)],
      catalog: (idOrSlug: string | number) => ['labels', 'catalog', String(idOrSlug)],
    },
    artists: {
      labels: (artistIdOrSlug: string | number) =>
        ['artists', 'labels', String(artistIdOrSlug)],
    },
  },
}))

// Import hooks after mocks are set up
import {
  useLabels,
  useLabel,
  useArtistLabels,
  useLabelRoster,
  useLabelCatalog,
} from './useLabels'

describe('Label Hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  // ──────────────────────────────────────────────
  // useLabels
  // ──────────────────────────────────────────────

  describe('useLabels', () => {
    it('fetches labels list without filters', async () => {
      const mockResponse = {
        labels: [
          { id: 1, name: 'Label A', slug: 'label-a' },
          { id: 2, name: 'Label B', slug: 'label-b' },
        ],
        total: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useLabels(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/labels', {
        method: 'GET',
      })
      expect(result.current.data?.labels).toHaveLength(2)
    })

    it('includes status filter in query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ labels: [], total: 0 })

      const { result } = renderHook(
        () => useLabels({ status: 'active' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/labels?status=active', {
        method: 'GET',
      })
    })

    it('includes city and state filters', async () => {
      mockApiRequest.mockResolvedValueOnce({ labels: [], total: 0 })

      const { result } = renderHook(
        () => useLabels({ city: 'Phoenix', state: 'AZ' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('city=Phoenix')
      expect(calledUrl).toContain('state=AZ')
    })

    it('combines multiple filters', async () => {
      mockApiRequest.mockResolvedValueOnce({ labels: [], total: 0 })

      const { result } = renderHook(
        () => useLabels({ status: 'active', city: 'Mesa', state: 'AZ' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('status=active')
      expect(calledUrl).toContain('city=Mesa')
      expect(calledUrl).toContain('state=AZ')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useLabels(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })

    it('returns empty labels list', async () => {
      mockApiRequest.mockResolvedValueOnce({ labels: [], total: 0 })

      const { result } = renderHook(() => useLabels(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.labels).toHaveLength(0)
    })
  })

  // ──────────────────────────────────────────────
  // useLabel
  // ──────────────────────────────────────────────

  describe('useLabel', () => {
    it('fetches a single label by slug', async () => {
      const mockLabel = {
        id: 1,
        name: 'Sub Pop',
        slug: 'sub-pop',
        city: 'Seattle',
        state: 'WA',
      }
      mockApiRequest.mockResolvedValueOnce(mockLabel)

      const { result } = renderHook(
        () => useLabel({ idOrSlug: 'sub-pop' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/labels/sub-pop', {
        method: 'GET',
      })
      expect(result.current.data?.name).toBe('Sub Pop')
    })

    it('fetches a single label by numeric ID', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 5, name: 'Label' })

      const { result } = renderHook(
        () => useLabel({ idOrSlug: 5 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/labels/5', {
        method: 'GET',
      })
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () => useLabel({ idOrSlug: 'sub-pop', enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when idOrSlug is empty string', async () => {
      const { result } = renderHook(
        () => useLabel({ idOrSlug: '' }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when numeric ID is 0 or negative', async () => {
      const { result: result0 } = renderHook(
        () => useLabel({ idOrSlug: 0 }),
        { wrapper: createWrapper() }
      )
      const { result: resultNeg } = renderHook(
        () => useLabel({ idOrSlug: -1 }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result0.current.fetchStatus).toBe('idle')
      expect(resultNeg.current.fetchStatus).toBe('idle')
    })

    it('handles label not found error', async () => {
      const error = new Error('Label not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () => useLabel({ idOrSlug: 'nonexistent' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).message).toBe('Label not found')
    })
  })

  // ──────────────────────────────────────────────
  // useArtistLabels
  // ──────────────────────────────────────────────

  describe('useArtistLabels', () => {
    it('fetches labels for an artist by slug', async () => {
      const mockResponse = {
        labels: [{ id: 1, name: 'Label A' }],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useArtistLabels({ artistIdOrSlug: 'the-smile' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/artists/the-smile/labels', {
        method: 'GET',
      })
    })

    it('fetches labels for an artist by numeric ID', async () => {
      mockApiRequest.mockResolvedValueOnce({ labels: [] })

      const { result } = renderHook(
        () => useArtistLabels({ artistIdOrSlug: 42 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/artists/42/labels', {
        method: 'GET',
      })
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () => useArtistLabels({ artistIdOrSlug: 'the-smile', enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when artistIdOrSlug is empty string', async () => {
      const { result } = renderHook(
        () => useArtistLabels({ artistIdOrSlug: '' }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when numeric ID is 0', async () => {
      const { result } = renderHook(
        () => useArtistLabels({ artistIdOrSlug: 0 }),
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
        () => useArtistLabels({ artistIdOrSlug: 'the-smile' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  // ──────────────────────────────────────────────
  // useLabelRoster
  // ──────────────────────────────────────────────

  describe('useLabelRoster', () => {
    it('fetches artists on a label by slug', async () => {
      const mockResponse = {
        artists: [
          { id: 1, name: 'Artist A' },
          { id: 2, name: 'Artist B' },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useLabelRoster({ labelIdOrSlug: 'sub-pop' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/labels/sub-pop/artists', {
        method: 'GET',
      })
      expect(result.current.data?.artists).toHaveLength(2)
    })

    it('fetches artists on a label by numeric ID', async () => {
      mockApiRequest.mockResolvedValueOnce({ artists: [] })

      const { result } = renderHook(
        () => useLabelRoster({ labelIdOrSlug: 10 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/labels/10/artists', {
        method: 'GET',
      })
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () => useLabelRoster({ labelIdOrSlug: 'sub-pop', enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when labelIdOrSlug is empty string', async () => {
      const { result } = renderHook(
        () => useLabelRoster({ labelIdOrSlug: '' }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when numeric ID is 0 or negative', async () => {
      const { result } = renderHook(
        () => useLabelRoster({ labelIdOrSlug: 0 }),
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
        () => useLabelRoster({ labelIdOrSlug: 'sub-pop' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  // ──────────────────────────────────────────────
  // useLabelCatalog
  // ──────────────────────────────────────────────

  describe('useLabelCatalog', () => {
    it('fetches releases on a label by slug', async () => {
      const mockResponse = {
        releases: [
          { id: 1, title: 'Album A' },
          { id: 2, title: 'Album B' },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useLabelCatalog({ labelIdOrSlug: 'sub-pop' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/labels/sub-pop/releases', {
        method: 'GET',
      })
      expect(result.current.data?.releases).toHaveLength(2)
    })

    it('fetches releases on a label by numeric ID', async () => {
      mockApiRequest.mockResolvedValueOnce({ releases: [] })

      const { result } = renderHook(
        () => useLabelCatalog({ labelIdOrSlug: 10 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/labels/10/releases', {
        method: 'GET',
      })
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () => useLabelCatalog({ labelIdOrSlug: 'sub-pop', enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when labelIdOrSlug is empty string', async () => {
      const { result } = renderHook(
        () => useLabelCatalog({ labelIdOrSlug: '' }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when numeric ID is 0 or negative', async () => {
      const { result } = renderHook(
        () => useLabelCatalog({ labelIdOrSlug: 0 }),
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
        () => useLabelCatalog({ labelIdOrSlug: 'sub-pop' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })
})
