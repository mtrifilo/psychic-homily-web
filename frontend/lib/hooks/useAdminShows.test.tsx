import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateShows = vi.fn()

// Mock the api module
vi.mock('../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      SHOWS: {
        PENDING: '/admin/shows/pending',
        REJECTED: '/admin/shows/rejected',
        APPROVE: (showId: number) => `/admin/shows/${showId}/approve`,
        REJECT: (showId: number) => `/admin/shows/${showId}/reject`,
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('../queryClient', () => ({
  createInvalidateQueries: () => ({
    shows: mockInvalidateShows,
  }),
}))

// Import hooks after mocks are set up
import {
  usePendingShows,
  useRejectedShows,
  useApproveShow,
  useRejectShow,
  adminQueryKeys,
} from './useAdminShows'

// Helper to create wrapper with specific query client
function createWrapperWithClient(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

describe('useAdminShows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShows.mockReset()
  })

  describe('adminQueryKeys', () => {
    it('generates correct query key for pending shows', () => {
      const key = adminQueryKeys.pendingShows(50, 0)
      expect(key).toEqual(['admin', 'shows', 'pending', { limit: 50, offset: 0 }])
    })

    it('generates correct query key for rejected shows', () => {
      const key = adminQueryKeys.rejectedShows(50, 0, 'search term')
      expect(key).toEqual([
        'admin',
        'shows',
        'rejected',
        { limit: 50, offset: 0, search: 'search term' },
      ])
    })

    it('generates query key for rejected shows without search', () => {
      const key = adminQueryKeys.rejectedShows(25, 10)
      expect(key).toEqual([
        'admin',
        'shows',
        'rejected',
        { limit: 25, offset: 10, search: undefined },
      ])
    })
  })

  describe('usePendingShows', () => {
    it('fetches pending shows with default options', async () => {
      const mockResponse = {
        shows: [
          { id: 1, title: 'Pending Show 1', status: 'pending' },
          { id: 2, title: 'Pending Show 2', status: 'pending' },
        ],
        total: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => usePendingShows(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/shows/pending?limit=50&offset=0'
      )
      expect(result.current.data?.shows).toHaveLength(2)
    })

    it('supports custom limit and offset', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () => usePendingShows({ limit: 25, offset: 50 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/shows/pending?limit=25&offset=50'
      )
    })

    it('handles authentication error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => usePendingShows(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Forbidden')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => usePendingShows(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useRejectedShows', () => {
    it('fetches rejected shows with default options', async () => {
      const mockResponse = {
        shows: [
          { id: 1, title: 'Rejected Show 1', status: 'rejected' },
        ],
        total: 1,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useRejectedShows(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/shows/rejected?limit=50&offset=0'
      )
    })

    it('supports search filter', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () => useRejectedShows({ search: 'test show' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest.mock.calls[0][0]).toContain('search=test+show')
    })

    it('supports custom limit, offset, and search', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () => useRejectedShows({ limit: 10, offset: 20, search: 'query' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('limit=10')
      expect(calledUrl).toContain('offset=20')
      expect(calledUrl).toContain('search=query')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useRejectedShows(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useApproveShow', () => {
    it('approves a show with verifyVenues true', async () => {
      const mockResponse = { id: 123, status: 'approved' }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const queryClient = createTestQueryClient()
      const { result } = renderHook(() => useApproveShow(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate({ showId: 123, verifyVenues: true })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/shows/123/approve',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ verify_venues: true }),
        })
      )
    })

    it('approves a show with verifyVenues false', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 456, status: 'approved' })

      const { result } = renderHook(() => useApproveShow(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ showId: 456, verifyVenues: false })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/shows/456/approve',
        expect.objectContaining({
          body: JSON.stringify({ verify_venues: false }),
        })
      )
    })

    it('invalidates queries on success', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 789 })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useApproveShow(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate({ showId: 789, verifyVenues: true })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      // Should invalidate pending shows
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['admin', 'shows', 'pending'],
      })
      // Should invalidate public shows
      expect(mockInvalidateShows).toHaveBeenCalled()
    })

    it('handles approval errors', async () => {
      const error = new Error('Show not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useApproveShow(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ showId: 999, verifyVenues: true })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Show not found')
    })

    it('handles unauthorized error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useApproveShow(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ showId: 123, verifyVenues: true })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useRejectShow', () => {
    it('rejects a show with a reason', async () => {
      const mockResponse = { id: 123, status: 'rejected' }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useRejectShow(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ showId: 123, reason: 'Duplicate show' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/shows/123/reject',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ reason: 'Duplicate show' }),
        })
      )
    })

    it('invalidates pending shows on success', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 456 })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useRejectShow(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate({ showId: 456, reason: 'Invalid venue' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['admin', 'shows', 'pending'],
      })
    })

    it('handles rejection errors', async () => {
      const error = new Error('Show not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useRejectShow(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ showId: 999, reason: 'Test' })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })

    it('handles unauthorized error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useRejectShow(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ showId: 123, reason: 'Test' })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })
})
