import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { radioQueryKeys } from '../api'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useRadioStation } from './useRadioStation'

const BASE = 'http://localhost:8080'

describe('useRadioStation', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('uses the station(slug) query key', () => {
    expect(radioQueryKeys.station('kexp')).toEqual(['radio-stations', 'kexp'])
  })

  it('fetches a single station by slug', async () => {
    const mockResponse = { id: 1, name: 'KEXP', slug: 'kexp' }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useRadioStation('kexp'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(`${BASE}/radio-stations/kexp`, {
      method: 'GET',
    })
    expect(result.current.data).toEqual(mockResponse)
  })

  it('does not fetch when slug is empty', () => {
    const { result } = renderHook(() => useRadioStation(''), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('starts in a loading state before the request resolves', () => {
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))
    const { result } = renderHook(() => useRadioStation('kexp'), {
      wrapper: createWrapper(),
    })
    expect(result.current.isLoading).toBe(true)
  })

  it('surfaces a not-found error', async () => {
    const error = new Error('Station not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useRadioStation('nope'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Station not found')
  })
})
