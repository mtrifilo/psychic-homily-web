import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockApiRequest = vi.fn()
const mockInvalidateArtistReports = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      ARTIST_REPORTS: {
        LIST: '/admin/artist-reports',
        DISMISS: (reportId: string | number) => `/admin/artist-reports/${reportId}/dismiss`,
        RESOLVE: (reportId: string | number) => `/admin/artist-reports/${reportId}/resolve`,
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    artistReports: {
      pending: (limit: number, offset: number) =>
        ['artistReports', 'pending', { limit, offset }],
    },
  },
  createInvalidateQueries: () => ({
    artistReports: mockInvalidateArtistReports,
  }),
}))

import {
  usePendingArtistReports,
  useDismissArtistReport,
  useResolveArtistReport,
} from './useAdminArtistReports'

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }
}

describe('usePendingArtistReports', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches pending artist reports with defaults', async () => {
    mockApiRequest.mockResolvedValueOnce({ reports: [], total: 0 })

    const { result } = renderHook(() => usePendingArtistReports(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/admin/artist-reports')
    expect(url).toContain('limit=50')
    expect(url).toContain('offset=0')
  })

  it('uses custom pagination', async () => {
    mockApiRequest.mockResolvedValueOnce({ reports: [], total: 0 })

    const { result } = renderHook(
      () => usePendingArtistReports({ limit: 10, offset: 20 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('limit=10')
    expect(url).toContain('offset=20')
  })
})

describe('useDismissArtistReport', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateArtistReports.mockReset()
  })

  it('dismisses a report with POST', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, status: 'dismissed' })

    const { result } = renderHook(() => useDismissArtistReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 1, notes: 'Not actionable' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/artist-reports/1/dismiss',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ notes: 'Not actionable' }),
      })
    )
  })

  it('dismisses without notes', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, status: 'dismissed' })

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

  it('invalidates reports on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1 })

    const { result } = renderHook(() => useDismissArtistReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 1 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateArtistReports).toHaveBeenCalled()
  })
})

describe('useResolveArtistReport', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateArtistReports.mockReset()
  })

  it('resolves a report with POST', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, status: 'resolved' })

    const { result } = renderHook(() => useResolveArtistReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 1, notes: 'Fixed' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/artist-reports/1/resolve',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ notes: 'Fixed' }),
      })
    )
  })

  it('invalidates reports on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1 })

    const { result } = renderHook(() => useResolveArtistReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 1 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateArtistReports).toHaveBeenCalled()
  })
})
