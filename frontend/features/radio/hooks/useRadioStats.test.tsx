import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { radioQueryKeys } from '../api'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useRadioStats } from './useRadioStats'

const BASE = 'http://localhost:8080'

describe('useRadioStats', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('uses the stats() query key', () => {
    expect(radioQueryKeys.stats()).toEqual(['radio', 'stats'])
  })

  it('fetches the stats endpoint', async () => {
    const mockResponse = {
      total_stations: 3,
      total_shows: 12,
      total_episodes: 400,
      total_plays: 9000,
      matched_plays: 6000,
      unique_artists: 1500,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useRadioStats(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(`${BASE}/radio/stats`, {
      method: 'GET',
    })
    expect(result.current.data).toEqual(mockResponse)
  })

  it('starts in a loading state before the request resolves', () => {
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))
    const { result } = renderHook(() => useRadioStats(), {
      wrapper: createWrapper(),
    })
    expect(result.current.isLoading).toBe(true)
  })

  it('surfaces API errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useRadioStats(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Server error')
  })
})
