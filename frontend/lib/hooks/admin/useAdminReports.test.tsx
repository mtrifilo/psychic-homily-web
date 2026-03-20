import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateShowReports = vi.fn()
const mockInvalidateShows = vi.fn()

// Mock the api module
vi.mock('../../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      REPORTS: {
        LIST: '/admin/reports',
        DISMISS: (reportId: number) => `/admin/reports/${reportId}/dismiss`,
        RESOLVE: (reportId: number) => `/admin/reports/${reportId}/resolve`,
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('../../queryClient', () => ({
  queryKeys: {
    showReports: {
      pending: (limit: number, offset: number) => [
        'showReports',
        'pending',
        { limit, offset },
      ],
    },
  },
  createInvalidateQueries: () => ({
    showReports: mockInvalidateShowReports,
    shows: mockInvalidateShows,
  }),
}))

// Import hooks after mocks are set up
import {
  usePendingReports,
  useDismissReport,
  useResolveReport,
} from './useAdminReports'

// Helper to create wrapper with specific query client
function createWrapperWithClient(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

describe('useAdminReports', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShowReports.mockReset()
    mockInvalidateShows.mockReset()
  })

  describe('usePendingReports', () => {
    it('fetches pending reports with default options', async () => {
      const mockResponse = {
        reports: [
          {
            id: 1,
            show_id: 10,
            report_type: 'cancelled',
            status: 'pending',
            created_at: '2026-03-19T10:00:00Z',
            updated_at: '2026-03-19T10:00:00Z',
          },
          {
            id: 2,
            show_id: 20,
            report_type: 'sold_out',
            status: 'pending',
            created_at: '2026-03-18T10:00:00Z',
            updated_at: '2026-03-18T10:00:00Z',
          },
        ],
        total: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => usePendingReports(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/reports?limit=50&offset=0',
        { method: 'GET' }
      )
      expect(result.current.data?.reports).toHaveLength(2)
      expect(result.current.data?.total).toBe(2)
    })

    it('supports custom limit and offset', async () => {
      mockApiRequest.mockResolvedValueOnce({ reports: [], total: 0 })

      const { result } = renderHook(
        () => usePendingReports({ limit: 25, offset: 50 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/reports?limit=25&offset=50',
        { method: 'GET' }
      )
    })

    it('handles empty reports list', async () => {
      mockApiRequest.mockResolvedValueOnce({ reports: [], total: 0 })

      const { result } = renderHook(() => usePendingReports(), {
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

      const { result } = renderHook(() => usePendingReports(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })

    it('handles authentication error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => usePendingReports(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Forbidden')
    })
  })

  describe('useDismissReport', () => {
    it('dismisses a report without notes', async () => {
      const mockResponse = {
        id: 1,
        show_id: 10,
        report_type: 'cancelled',
        status: 'dismissed',
        created_at: '2026-03-19T10:00:00Z',
        updated_at: '2026-03-19T11:00:00Z',
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useDismissReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 1 })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/reports/1/dismiss',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({}),
        })
      )
    })

    it('dismisses a report with notes', async () => {
      const mockResponse = {
        id: 2,
        show_id: 20,
        status: 'dismissed',
        admin_notes: 'Spam report',
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useDismissReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 2, notes: 'Spam report' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/reports/2/dismiss',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ notes: 'Spam report' }),
        })
      )
    })

    it('invalidates show reports on success', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 3, status: 'dismissed' })

      const { result } = renderHook(() => useDismissReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 3 })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockInvalidateShowReports).toHaveBeenCalled()
    })

    it('handles dismiss error', async () => {
      const error = new Error('Report not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useDismissReport(), {
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

      const { result } = renderHook(() => useDismissReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 1 })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useResolveReport', () => {
    it('resolves a report without setting show flag', async () => {
      const mockResponse = {
        id: 1,
        show_id: 10,
        status: 'resolved',
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useResolveReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 1 })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/reports/1/resolve',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ set_show_flag: false }),
        })
      )
    })

    it('resolves a report with setShowFlag true', async () => {
      const mockResponse = {
        id: 2,
        show_id: 20,
        status: 'resolved',
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useResolveReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 2, setShowFlag: true })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/reports/2/resolve',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ set_show_flag: true }),
        })
      )
    })

    it('resolves a report with notes and setShowFlag', async () => {
      const mockResponse = {
        id: 3,
        show_id: 30,
        status: 'resolved',
        admin_notes: 'Verified cancelled',
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useResolveReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          reportId: 3,
          notes: 'Verified cancelled',
          setShowFlag: true,
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/reports/3/resolve',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            set_show_flag: true,
            notes: 'Verified cancelled',
          }),
        })
      )
    })

    it('invalidates show reports and shows on success', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 4, status: 'resolved' })

      const { result } = renderHook(() => useResolveReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 4, setShowFlag: true })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockInvalidateShowReports).toHaveBeenCalled()
      expect(mockInvalidateShows).toHaveBeenCalled()
    })

    it('handles resolve error', async () => {
      const error = new Error('Report not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useResolveReport(), {
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

      const { result } = renderHook(() => useResolveReport(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ reportId: 1, setShowFlag: true })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })
})
