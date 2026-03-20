import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateArtistReports = vi.fn()

// Mock the api module
vi.mock('../../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      ARTIST_REPORTS: {
        LIST: '/admin/artist-reports',
        DISMISS: (reportId: number) =>
          `/admin/artist-reports/${reportId}/dismiss`,
        RESOLVE: (reportId: number) =>
          `/admin/artist-reports/${reportId}/resolve`,
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('../../queryClient', () => ({
  queryKeys: {
    artistReports: {
      pending: (limit: number, offset: number) => [
        'artistReports',
        'pending',
        { limit, offset },
      ],
    },
  },
  createInvalidateQueries: () => ({
    artistReports: mockInvalidateArtistReports,
  }),
}))

// Import hooks after mocks are set up
import {
  usePendingArtistReports,
  useDismissArtistReport,
  useResolveArtistReport,
} from './useAdminArtistReports'

// Helper to create wrapper with specific query client
function createWrapperWithClient(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

describe('useAdminArtistReports', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateArtistReports.mockReset()
  })

  describe('usePendingArtistReports', () => {
    it('fetches pending artist reports with default options', async () => {
      const mockResponse = {
        reports: [
          {
            id: 1,
            artist_id: 10,
            report_type: 'duplicate',
            status: 'pending',
            created_at: '2026-03-19T10:00:00Z',
            updated_at: '2026-03-19T10:00:00Z',
          },
          {
            id: 2,
            artist_id: 20,
            report_type: 'incorrect_info',
            status: 'pending',
            created_at: '2026-03-18T09:00:00Z',
            updated_at: '2026-03-18T09:00:00Z',
          },
        ],
        total: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => usePendingArtistReports(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/artist-reports?limit=50&offset=0',
        { method: 'GET' }
      )
      expect(result.current.data?.reports).toHaveLength(2)
      expect(result.current.data?.total).toBe(2)
    })

    it('supports custom limit and offset', async () => {
      mockApiRequest.mockResolvedValueOnce({ reports: [], total: 0 })

      const { result } = renderHook(
        () => usePendingArtistReports({ limit: 10, offset: 20 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/artist-reports?limit=10&offset=20',
        { method: 'GET' }
      )
    })

    it('handles empty reports list', async () => {
      mockApiRequest.mockResolvedValueOnce({ reports: [], total: 0 })

      const { result } = renderHook(() => usePendingArtistReports(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.reports).toHaveLength(0)
      expect(result.current.data?.total).toBe(0)
    })

    it('handles API error', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => usePendingArtistReports(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })

    it('handles authentication error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => usePendingArtistReports(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Forbidden')
    })
  })

  describe('useDismissArtistReport', () => {
    it('dismisses a report without notes', async () => {
      const mockResponse = {
        id: 1,
        artist_id: 10,
        status: 'dismissed',
        created_at: '2026-03-19T10:00:00Z',
        updated_at: '2026-03-19T11:00:00Z',
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useDismissArtistReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 1 })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/artist-reports/1/dismiss',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({}),
        })
      )
    })

    it('dismisses a report with notes', async () => {
      const mockResponse = {
        id: 2,
        artist_id: 20,
        status: 'dismissed',
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useDismissArtistReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          reportId: 2,
          notes: 'Not a duplicate, different artist',
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/artist-reports/2/dismiss',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            notes: 'Not a duplicate, different artist',
          }),
        })
      )
    })

    it('invalidates artist reports on success', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 3, status: 'dismissed' })

      const { result } = renderHook(() => useDismissArtistReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 3 })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockInvalidateArtistReports).toHaveBeenCalled()
    })

    it('handles dismiss error', async () => {
      const error = new Error('Report not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useDismissArtistReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 999 })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Report not found')
    })

    it('handles unauthorized error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useDismissArtistReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 1 })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useResolveArtistReport', () => {
    it('resolves a report without notes', async () => {
      const mockResponse = {
        id: 1,
        artist_id: 10,
        status: 'resolved',
        created_at: '2026-03-19T10:00:00Z',
        updated_at: '2026-03-19T12:00:00Z',
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useResolveArtistReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 1 })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/artist-reports/1/resolve',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({}),
        })
      )
    })

    it('resolves a report with notes', async () => {
      const mockResponse = {
        id: 2,
        artist_id: 20,
        status: 'resolved',
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useResolveArtistReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          reportId: 2,
          notes: 'Merged duplicate entries',
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/artist-reports/2/resolve',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ notes: 'Merged duplicate entries' }),
        })
      )
    })

    it('invalidates artist reports on success', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 4, status: 'resolved' })

      const { result } = renderHook(() => useResolveArtistReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 4 })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockInvalidateArtistReports).toHaveBeenCalled()
    })

    it('handles resolve error', async () => {
      const error = new Error('Report not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useResolveArtistReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 999 })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Report not found')
    })

    it('handles server error', async () => {
      const error = new Error('Internal server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useResolveArtistReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 1, notes: 'Action taken' })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })
})
