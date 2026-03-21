import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()
const mockInvalidateShowReports = vi.fn()
const mockInvalidateShows = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      REPORTS: {
        LIST: '/admin/reports',
        DISMISS: (reportId: string | number) => `/admin/reports/${reportId}/dismiss`,
        RESOLVE: (reportId: string | number) => `/admin/reports/${reportId}/resolve`,
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    showReports: {
      pending: (limit: number, offset: number) =>
        ['showReports', 'pending', { limit, offset }],
    },
  },
  createInvalidateQueries: () => ({
    showReports: mockInvalidateShowReports,
    shows: mockInvalidateShows,
  }),
}))

import {
  usePendingReports,
  useDismissReport,
  useResolveReport,
} from './useAdminReports'


describe('usePendingReports', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches pending show reports with defaults', async () => {
    mockApiRequest.mockResolvedValueOnce({ reports: [], total: 0 })

    const { result } = renderHook(() => usePendingReports(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/admin/reports')
    expect(url).toContain('limit=50')
    expect(url).toContain('offset=0')
  })

  it('uses custom pagination', async () => {
    mockApiRequest.mockResolvedValueOnce({ reports: [], total: 0 })

    const { result } = renderHook(
      () => usePendingReports({ limit: 10, offset: 30 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('limit=10')
    expect(url).toContain('offset=30')
  })
})

describe('useDismissReport', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShowReports.mockReset()
  })

  it('dismisses a report with POST', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, status: 'dismissed' })

    const { result } = renderHook(() => useDismissReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 1, notes: 'Spam' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/reports/1/dismiss',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ notes: 'Spam' }),
      })
    )
  })

  it('invalidates show reports on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1 })

    const { result } = renderHook(() => useDismissReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 1 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateShowReports).toHaveBeenCalled()
  })
})

describe('useResolveReport', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShowReports.mockReset()
    mockInvalidateShows.mockReset()
  })

  it('resolves a report with POST', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, status: 'resolved' })

    const { result } = renderHook(() => useResolveReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 1, notes: 'Fixed', setShowFlag: true })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/reports/1/resolve',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ set_show_flag: true, notes: 'Fixed' }),
      })
    )
  })

  it('defaults setShowFlag to false', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, status: 'resolved' })

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
        body: JSON.stringify({ set_show_flag: false }),
      })
    )
  })

  it('invalidates show reports and shows on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1 })

    const { result } = renderHook(() => useResolveReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 1 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateShowReports).toHaveBeenCalled()
    expect(mockInvalidateShows).toHaveBeenCalled()
  })
})
