import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()
const mockInvalidateShowReports = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/features/shows/api', () => ({
  showEndpoints: {
    REPORT: (showId: string | number) => `/shows/${showId}/report`,
    MY_REPORT: (showId: string | number) => `/shows/${showId}/my-report`,
  },
}))

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

import { useMyShowReport, useReportShow } from './useShowReports'


describe('useMyShowReport', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches user report for a show by numeric ID', async () => {
    mockApiRequest.mockResolvedValueOnce({ has_report: true, report_type: 'wrong_date' })

    const { result } = renderHook(() => useMyShowReport(42), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/shows/42/my-report', { method: 'GET' })
  })

  it('fetches user report for a show by string ID', async () => {
    mockApiRequest.mockResolvedValueOnce({ has_report: false })

    const { result } = renderHook(() => useMyShowReport('my-slug'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/shows/my-slug/my-report', { method: 'GET' })
  })

  it('does not fetch when showId is null', () => {
    const { result } = renderHook(() => useMyShowReport(null), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('handles API errors', async () => {
    const error = new Error('Not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useMyShowReport(999), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})

describe('useReportShow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShowReports.mockReset()
  })

  it('reports a show with POST', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, report_type: 'wrong_date' })

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        showId: 42,
        reportType: 'wrong_date',
        details: 'The date is March 20, not March 19',
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows/42/report',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          report_type: 'wrong_date',
          details: 'The date is March 20, not March 19',
        }),
      })
    )
  })

  it('sends null for details when not provided', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1 })

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        showId: 42,
        reportType: 'duplicate',
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows/42/report',
      expect.objectContaining({
        body: JSON.stringify({
          report_type: 'duplicate',
          details: null,
        }),
      })
    )
  })

  it('invalidates show reports on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1 })

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 42, reportType: 'wrong_date' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateShowReports).toHaveBeenCalled()
  })

  it('handles report errors', async () => {
    const error = new Error('Already reported')
    Object.assign(error, { status: 409 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 42, reportType: 'wrong_date' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})
