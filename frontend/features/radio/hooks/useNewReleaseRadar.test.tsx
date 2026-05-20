import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { radioQueryKeys } from '../api'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useNewReleaseRadar } from './useNewReleaseRadar'

const BASE = 'http://localhost:8080'

describe('useNewReleaseRadar', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('carries stationId/limit in the newReleases() query key', () => {
    expect(radioQueryKeys.newReleases({ stationId: 7, limit: 20 })).toEqual([
      'radio',
      'new-releases',
      { stationId: 7, limit: 20 },
    ])
  })

  it('fetches with the default limit and no station filter', async () => {
    const mockResponse = {
      releases: [{ artist_name: 'Gatecreeper', play_count: 4, station_count: 2 }],
      count: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useNewReleaseRadar(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // No stationId → only limit param.
    expect(mockApiRequest).toHaveBeenCalledWith(
      `${BASE}/radio/new-releases?limit=20`,
      { method: 'GET' }
    )
    expect(result.current.data).toEqual(mockResponse)
  })

  it('includes station_id when a station filter is supplied', async () => {
    mockApiRequest.mockResolvedValueOnce({ releases: [], count: 0 })

    const { result } = renderHook(
      () => useNewReleaseRadar({ stationId: 7, limit: 5 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('station_id=7')
    expect(calledUrl).toContain('limit=5')
  })

  it('does not fetch when explicitly disabled', () => {
    const { result } = renderHook(() => useNewReleaseRadar({ enabled: false }), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('starts in a loading state before the request resolves', () => {
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))
    const { result } = renderHook(() => useNewReleaseRadar(), {
      wrapper: createWrapper(),
    })
    expect(result.current.isLoading).toBe(true)
  })

  it('surfaces API errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useNewReleaseRadar(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Server error')
  })
})
