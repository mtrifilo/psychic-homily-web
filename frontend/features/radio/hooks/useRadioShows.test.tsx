import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { radioQueryKeys } from '../api'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useRadioShows } from './useRadioShows'

const BASE = 'http://localhost:8080'

describe('useRadioShows', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('wraps stationId in the shows() query key', () => {
    expect(radioQueryKeys.shows(7)).toEqual(['radio-shows', { stationId: 7 }])
  })

  it('fetches shows scoped to a station via station_id query param', async () => {
    const mockResponse = {
      shows: [{ id: 1, name: 'The Drummer Show', slug: 'drummer' }],
      count: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useRadioShows(7), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      `${BASE}/radio-shows?station_id=7`,
      { method: 'GET' }
    )
    expect(result.current.data).toEqual(mockResponse)
  })

  it('does not fetch when stationId is omitted', () => {
    const { result } = renderHook(() => useRadioShows(), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('starts in a loading state before the request resolves', () => {
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))
    const { result } = renderHook(() => useRadioShows(7), {
      wrapper: createWrapper(),
    })
    expect(result.current.isLoading).toBe(true)
  })

  it('surfaces API errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useRadioShows(7), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Server error')
  })
})
