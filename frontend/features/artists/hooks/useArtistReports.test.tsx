import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock the feature api module
vi.mock('@/features/artists/api', () => ({
  artistEndpoints: {
    MY_REPORT: (artistId: string | number) => `/artists/${artistId}/my-report`,
  },
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    artistReports: {
      myReport: (artistId: string | number) =>
        ['artistReports', 'myReport', String(artistId)],
    },
  },
}))

// Import hooks after mocks are set up
import { useMyArtistReport } from './useArtistReports'

describe('useMyArtistReport', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it("fetches the user's existing report for an artist", async () => {
    // PSY-1633: the endpoint answers with the ENTITY report shape
    // (entity_type / entity_id), not the retired artist_id shape.
    const mockResponse = {
      report: {
        id: 1,
        entity_type: 'artist',
        entity_id: 42,
        entity_name: 'Test Artist',
        entity_slug: 'test-artist',
        reported_by: 7,
        reporter_username: null,
        report_type: 'inaccurate',
        details: 'Wrong city listed',
        status: 'pending',
        created_at: '2025-03-01T00:00:00Z',
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
    expect(result.current.data?.report?.entity_type).toBe('artist')
    expect(result.current.data?.report?.entity_id).toBe(42)
  })

  it('returns null report when user has not reported', async () => {
    mockApiRequest.mockResolvedValueOnce({ report: null })

    const { result } = renderHook(() => useMyArtistReport(42), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Hook returns null report from API when no report exists
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
