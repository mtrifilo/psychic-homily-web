import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { radioQueryKeys } from '../api'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useRadioStations } from './useRadioStations'

const BASE = 'http://localhost:8080'

describe('useRadioStations', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('uses the stations() query key', () => {
    expect(radioQueryKeys.stations()).toEqual(['radio-stations'])
  })

  it('fetches the station list on the stations endpoint', async () => {
    const mockResponse = {
      stations: [{ id: 1, name: 'KEXP', slug: 'kexp' }],
      count: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useRadioStations(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(`${BASE}/radio-stations`, {
      method: 'GET',
    })
    expect(result.current.data).toEqual(mockResponse)
  })

  it('starts in a loading state before the request resolves', () => {
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))
    const { result } = renderHook(() => useRadioStations(), {
      wrapper: createWrapper(),
    })
    expect(result.current.isLoading).toBe(true)
  })

  it('surfaces API errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useRadioStations(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Server error')
  })
})
