import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateShowReports = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    SHOWS: {
      REPORT: (showId: string | number) => `/shows/${showId}/report`,
      MY_REPORT: (showId: string | number) => `/shows/${showId}/my-report`,
    },
  },
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    showReports: {
      myReport: (showId: string) => ['showReports', 'myReport', showId],
    },
  },
  createInvalidateQueries: () => ({
    showReports: mockInvalidateShowReports,
  }),
}))

// Import hooks after mocks are set up
import { useMyShowReport, useReportShow } from './useShowReports'

// Helper to create wrapper with query client
function createWrapper(queryClient?: QueryClient) {
  const qc = queryClient ?? new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    )
  }
}

describe('useMyShowReport', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches the user report for a show when showId is a number', async () => {
    const mockReport = {
      report: {
        id: 1,
        show_id: 42,
        report_type: 'cancelled' as const,
        details: 'Show was cancelled',
        status: 'pending' as const,
        created_at: '2025-01-15T00:00:00Z',
        updated_at: '2025-01-15T00:00:00Z',
      },
    }
    mockApiRequest.mockResolvedValueOnce(mockReport)

    const { result } = renderHook(() => useMyShowReport(42), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/shows/42/my-report', {
      method: 'GET',
    })
    expect(result.current.data?.report?.report_type).toBe('cancelled')
  })

  it('fetches the user report for a show when showId is a string', async () => {
    mockApiRequest.mockResolvedValueOnce({ report: null })

    const { result } = renderHook(() => useMyShowReport('123'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/shows/123/my-report', {
      method: 'GET',
    })
  })

  it('does not fetch when showId is null', () => {
    const { result } = renderHook(() => useMyShowReport(null), {
      wrapper: createWrapper(),
    })

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('returns null report when user has not reported', async () => {
    mockApiRequest.mockResolvedValueOnce({ report: null })

    const { result } = renderHook(() => useMyShowReport(10), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.report).toBeNull()
  })

  it('handles API errors', async () => {
    const error = new Error('Not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useMyShowReport(999), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })

  it('returns all report fields when present', async () => {
    const mockReport = {
      report: {
        id: 5,
        show_id: 20,
        report_type: 'sold_out' as const,
        details: 'Sold out on Ticketmaster',
        status: 'resolved' as const,
        admin_notes: 'Confirmed and updated',
        reviewed_by: 1,
        reviewed_at: '2025-01-16T10:00:00Z',
        created_at: '2025-01-15T00:00:00Z',
        updated_at: '2025-01-16T10:00:00Z',
      },
    }
    mockApiRequest.mockResolvedValueOnce(mockReport)

    const { result } = renderHook(() => useMyShowReport(20), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const report = result.current.data?.report
    expect(report?.id).toBe(5)
    expect(report?.status).toBe('resolved')
    expect(report?.admin_notes).toBe('Confirmed and updated')
    expect(report?.reviewed_by).toBe(1)
  })
})

describe('useReportShow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShowReports.mockReset()
  })

  it('reports a show with cancelled type', async () => {
    const mockResponse = {
      id: 1,
      show_id: 42,
      report_type: 'cancelled' as const,
      details: null,
      status: 'pending' as const,
      created_at: '2025-01-15T00:00:00Z',
      updated_at: '2025-01-15T00:00:00Z',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 42, reportType: 'cancelled' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/shows/42/report', {
      method: 'POST',
      body: JSON.stringify({
        report_type: 'cancelled',
        details: null,
      }),
    })
  })

  it('reports a show with sold_out type', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 2,
      show_id: 10,
      report_type: 'sold_out',
      details: null,
      status: 'pending',
      created_at: '2025-01-15T00:00:00Z',
      updated_at: '2025-01-15T00:00:00Z',
    })

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 10, reportType: 'sold_out' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const sentBody = JSON.parse(mockApiRequest.mock.calls[0][1].body)
    expect(sentBody.report_type).toBe('sold_out')
  })

  it('reports a show with inaccurate type and details', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 3,
      show_id: 5,
      report_type: 'inaccurate',
      details: 'Wrong venue listed',
      status: 'pending',
      created_at: '2025-01-15T00:00:00Z',
      updated_at: '2025-01-15T00:00:00Z',
    })

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        showId: 5,
        reportType: 'inaccurate',
        details: 'Wrong venue listed',
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const sentBody = JSON.parse(mockApiRequest.mock.calls[0][1].body)
    expect(sentBody.report_type).toBe('inaccurate')
    expect(sentBody.details).toBe('Wrong venue listed')
  })

  it('sends null for details when not provided', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 4,
      show_id: 7,
      report_type: 'cancelled',
      details: null,
      status: 'pending',
      created_at: '2025-01-15T00:00:00Z',
      updated_at: '2025-01-15T00:00:00Z',
    })

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 7, reportType: 'cancelled' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const sentBody = JSON.parse(mockApiRequest.mock.calls[0][1].body)
    expect(sentBody.details).toBeNull()
  })

  it('sends null for details when details is empty string', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 5,
      show_id: 8,
      report_type: 'cancelled',
      details: null,
      status: 'pending',
      created_at: '2025-01-15T00:00:00Z',
      updated_at: '2025-01-15T00:00:00Z',
    })

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 8, reportType: 'cancelled', details: '' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const sentBody = JSON.parse(mockApiRequest.mock.calls[0][1].body)
    expect(sentBody.details).toBeNull()
  })

  it('invalidates myReport and showReports on success', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 0 },
        mutations: { retry: false },
      },
    })

    // Spy on invalidateQueries
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    mockApiRequest.mockResolvedValueOnce({
      id: 6,
      show_id: 15,
      report_type: 'cancelled',
      details: null,
      status: 'pending',
      created_at: '2025-01-15T00:00:00Z',
      updated_at: '2025-01-15T00:00:00Z',
    })

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate({ showId: 15, reportType: 'cancelled' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Check that myReport for the specific show was invalidated
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['showReports', 'myReport', '15'],
    })

    // Check that the general showReports invalidation was called
    expect(mockInvalidateShowReports).toHaveBeenCalled()
  })

  it('handles 409 duplicate report error', async () => {
    const error = new Error('You have already reported this show')
    Object.assign(error, { status: 409 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 42, reportType: 'cancelled' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe(
      'You have already reported this show'
    )
  })

  it('handles 401 unauthorized error', async () => {
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, reportType: 'inaccurate' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe('Unauthorized')
  })

  it('does not invalidate queries on error', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Server error'))

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, reportType: 'cancelled' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(mockInvalidateShowReports).not.toHaveBeenCalled()
  })

  it('handles network errors', async () => {
    mockApiRequest.mockRejectedValueOnce(new TypeError('Failed to fetch'))

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, reportType: 'cancelled' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeInstanceOf(TypeError)
  })

  it('returns the report response data on success', async () => {
    const mockResponse = {
      id: 10,
      show_id: 50,
      report_type: 'inaccurate' as const,
      details: 'Wrong date',
      status: 'pending' as const,
      created_at: '2025-01-15T12:00:00Z',
      updated_at: '2025-01-15T12:00:00Z',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        showId: 50,
        reportType: 'inaccurate',
        details: 'Wrong date',
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.id).toBe(10)
    expect(result.current.data?.show_id).toBe(50)
    expect(result.current.data?.report_type).toBe('inaccurate')
    expect(result.current.data?.status).toBe('pending')
  })
})
