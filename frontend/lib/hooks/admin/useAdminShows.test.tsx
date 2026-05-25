import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
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
    SHOWS: {
      SET_SOLD_OUT: (showId: number) => `/shows/${showId}/sold-out`,
      SET_CANCELLED: (showId: number) => `/shows/${showId}/cancelled`,
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
  useSetShowSoldOut,
  useSetShowCancelled,
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

    it('appends venueId + source filters when provided', async () => {
      // Both filters are optional on the admin moderation surface — verify
      // they survive into the URL so the backend can scope correctly.
      mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

      const { result } = renderHook(
        () => usePendingShows({ venueId: 7, source: 'ical' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      const url = mockApiRequest.mock.calls[0][0] as string
      expect(url).toContain('venue_id=7')
      expect(url).toContain('source=ical')
    })

    it('does not fetch when enabled=false', () => {
      const { result } = renderHook(
        () => usePendingShows({ enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(result.current.fetchStatus).toBe('idle')
      expect(mockApiRequest).not.toHaveBeenCalled()
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

    it('surfaces fetch errors', async () => {
      mockApiRequest.mockRejectedValueOnce(new Error('Forbidden'))

      const { result } = renderHook(() => useRejectedShows(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).message).toBe('Forbidden')
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
      const mockResponse = { approved: 3, errors: [] as unknown[] }
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
      const mockResponse = { rejected: 2, errors: [] as unknown[] }
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

  describe('useSetShowSoldOut', () => {
    it('POSTs the sold-out toggle value (true)', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 1, is_sold_out: true })

      const { result } = renderHook(() => useSetShowSoldOut(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ showId: 1, value: true })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith(
        '/shows/1/sold-out',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ value: true }),
        })
      )
    })

    it('POSTs the sold-out toggle value (false) to clear', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 1, is_sold_out: false })

      const { result } = renderHook(() => useSetShowSoldOut(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ showId: 1, value: false })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith(
        '/shows/1/sold-out',
        expect.objectContaining({
          body: JSON.stringify({ value: false }),
        })
      )
    })

    it('invalidates the public shows scope on success', async () => {
      // Sold-out display is denormalised across show listings, so the
      // broad ['shows'] scope must invalidate. We assert via the shared
      // invalidateQueries.shows() spy (mocked at module level).
      mockApiRequest.mockResolvedValueOnce({ id: 1 })

      const { result } = renderHook(() => useSetShowSoldOut(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ showId: 1, value: true })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockInvalidateShows).toHaveBeenCalled()
    })

    it('surfaces errors', async () => {
      mockApiRequest.mockRejectedValueOnce(new Error('Forbidden'))

      const { result } = renderHook(() => useSetShowSoldOut(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ showId: 1, value: true })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).message).toBe('Forbidden')
      expect(mockInvalidateShows).not.toHaveBeenCalled()
    })
  })

  describe('useSetShowCancelled', () => {
    it('POSTs the cancelled toggle value', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 5, is_cancelled: true })

      const { result } = renderHook(() => useSetShowCancelled(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ showId: 5, value: true })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith(
        '/shows/5/cancelled',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ value: true }),
        })
      )
      expect(mockInvalidateShows).toHaveBeenCalled()
    })

    it('clears the cancelled flag with value=false', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 5, is_cancelled: false })

      const { result } = renderHook(() => useSetShowCancelled(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ showId: 5, value: false })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith(
        '/shows/5/cancelled',
        expect.objectContaining({
          body: JSON.stringify({ value: false }),
        })
      )
    })
  })
})
