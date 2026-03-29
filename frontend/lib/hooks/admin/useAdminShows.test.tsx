import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapper, createWrapperWithClient, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateShows = vi.fn()

// Mock the api module
vi.mock('../../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      SHOWS: {
        PENDING: '/admin/shows/pending',
        REJECTED: '/admin/shows/rejected',
        APPROVE: (showId: number) => `/admin/shows/${showId}/approve`,
        REJECT: (showId: number) => `/admin/shows/${showId}/reject`,
        BATCH_APPROVE: '/admin/shows/batch-approve',
        BATCH_REJECT: '/admin/shows/batch-reject',
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('../../queryClient', () => ({
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
  useBatchApproveShows,
  useBatchRejectShows,
  adminQueryKeys,
} from './useAdminShows'


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

  })

  describe('useBatchApproveShows', () => {
    it('batch approves shows', async () => {
      const mockResponse = { approved: 3, errors: [] }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useBatchApproveShows(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate([1, 2, 3])
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/shows/batch-approve',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ show_ids: [1, 2, 3] }),
        })
      )
    })

    it('invalidates queries on success', async () => {
      mockApiRequest.mockResolvedValueOnce({ approved: 2, errors: [] })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useBatchApproveShows(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate([1, 2])
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['admin', 'shows', 'pending'],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['admin', 'shows', 'rejected'],
      })
      expect(mockInvalidateShows).toHaveBeenCalled()
    })

  })

  describe('useBatchRejectShows', () => {
    it('batch rejects shows with reason and category', async () => {
      const mockResponse = { rejected: 2, errors: [] }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useBatchRejectShows(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          showIds: [4, 5],
          reason: 'Not music events',
          category: 'non_music',
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/shows/batch-reject',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            show_ids: [4, 5],
            reason: 'Not music events',
            category: 'non_music',
          }),
        })
      )
    })

    it('batch rejects without category', async () => {
      mockApiRequest.mockResolvedValueOnce({ rejected: 1, errors: [] })

      const { result } = renderHook(() => useBatchRejectShows(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          showIds: [6],
          reason: 'Duplicate',
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/shows/batch-reject',
        expect.objectContaining({
          body: JSON.stringify({
            show_ids: [6],
            reason: 'Duplicate',
          }),
        })
      )
    })

    it('invalidates queries on success', async () => {
      mockApiRequest.mockResolvedValueOnce({ rejected: 1, errors: [] })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useBatchRejectShows(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate({ showIds: [1], reason: 'Bad data' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['admin', 'shows', 'pending'],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['admin', 'stats'],
      })
    })

  })
})
