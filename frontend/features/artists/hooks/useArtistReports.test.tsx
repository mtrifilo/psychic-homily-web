import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateArtistReports = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ARTISTS: {
      REPORT: (artistId: string | number) => `/artists/${artistId}/report`,
      MY_REPORT: (artistId: string | number) => `/artists/${artistId}/my-report`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    artistReports: {
      myReport: (artistId: string | number) =>
        ['artistReports', 'myReport', String(artistId)],
    },
  },
  createInvalidateQueries: () => ({
    artistReports: mockInvalidateArtistReports,
  }),
}))

// Import hooks after mocks are set up
import { useMyArtistReport, useReportArtist } from './useArtistReports'

describe('useMyArtistReport', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches the user\'s existing report for an artist', async () => {
    const mockResponse = {
      report: {
        id: 1,
        artist_id: 42,
        report_type: 'inaccurate',
        details: 'Wrong city listed',
        status: 'pending',
        created_at: '2025-03-01T00:00:00Z',
        updated_at: '2025-03-01T00:00:00Z',
      },
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useMyArtistReport(42), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/artists/42/my-report', {
      method: 'GET',
    })
    expect(result.current.data?.report?.report_type).toBe('inaccurate')
    expect(result.current.data?.report?.status).toBe('pending')
  })

  it('returns null report when user has not reported', async () => {
    mockApiRequest.mockResolvedValueOnce({ report: null })

    const { result } = renderHook(() => useMyArtistReport(42), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.report).toBeNull()
  })

  it('does not fetch when artistId is null', () => {
    const { result } = renderHook(() => useMyArtistReport(null), {
      wrapper: createWrapper(),
    })

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('accepts string artistId', async () => {
    mockApiRequest.mockResolvedValueOnce({ report: null })

    const { result } = renderHook(() => useMyArtistReport('42'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/artists/42/my-report', {
      method: 'GET',
    })
  })

  it('handles API errors', async () => {
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useMyArtistReport(42), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe('Unauthorized')
  })

  it('accepts numeric artistId 0 as falsy — does not fetch', () => {
    const { result } = renderHook(() => useMyArtistReport(0), {
      wrapper: createWrapper(),
    })

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useReportArtist', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateArtistReports.mockReset()
  })

  it('creates a report and invalidates queries', async () => {
    const mockResponse = {
      id: 10,
      artist_id: 42,
      report_type: 'inaccurate',
      details: 'Wrong social links',
      status: 'pending',
      created_at: '2025-03-15T00:00:00Z',
      updated_at: '2025-03-15T00:00:00Z',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useReportArtist(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      const data = await result.current.mutateAsync({
        artistId: 42,
        reportType: 'inaccurate',
        details: 'Wrong social links',
      })
      expect(data.id).toBe(10)
      expect(data.report_type).toBe('inaccurate')
      expect(data.status).toBe('pending')
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/artists/42/report', {
      method: 'POST',
      body: JSON.stringify({
        report_type: 'inaccurate',
        details: 'Wrong social links',
      }),
    })
    expect(mockInvalidateArtistReports).toHaveBeenCalled()
  })

  it('creates a report without details', async () => {
    const mockResponse = {
      id: 11,
      artist_id: 42,
      report_type: 'removal_request',
      details: null,
      status: 'pending',
      created_at: '2025-03-15T00:00:00Z',
      updated_at: '2025-03-15T00:00:00Z',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useReportArtist(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      await result.current.mutateAsync({
        artistId: 42,
        reportType: 'removal_request',
      })
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/artists/42/report', {
      method: 'POST',
      body: JSON.stringify({
        report_type: 'removal_request',
        details: null,
      }),
    })
  })

  it('handles duplicate report error', async () => {
    const error = new Error('You have already reported this artist')
    Object.assign(error, { status: 409 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useReportArtist(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync({
          artistId: 42,
          reportType: 'inaccurate',
          details: 'Test',
        })
      } catch (e) {
        expect((e as Error).message).toBe(
          'You have already reported this artist'
        )
      }
    })

    expect(mockInvalidateArtistReports).not.toHaveBeenCalled()
  })

  it('handles server errors', async () => {
    const error = new Error('Internal server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useReportArtist(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync({
          artistId: 42,
          reportType: 'inaccurate',
        })
      } catch (e) {
        expect((e as Error).message).toBe('Internal server error')
      }
    })

    expect(mockInvalidateArtistReports).not.toHaveBeenCalled()
  })

  it('handles empty string details as null', async () => {
    const mockResponse = {
      id: 12,
      artist_id: 42,
      report_type: 'inaccurate',
      details: null,
      status: 'pending',
      created_at: '2025-03-15T00:00:00Z',
      updated_at: '2025-03-15T00:00:00Z',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useReportArtist(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      await result.current.mutateAsync({
        artistId: 42,
        reportType: 'inaccurate',
        details: '',
      })
    })

    // The hook sends `details || null`, so empty string becomes null
    expect(mockApiRequest).toHaveBeenCalledWith('/artists/42/report', {
      method: 'POST',
      body: JSON.stringify({
        report_type: 'inaccurate',
        details: null,
      }),
    })
  })
})
