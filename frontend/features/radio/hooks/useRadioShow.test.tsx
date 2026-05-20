import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { radioQueryKeys } from '../api'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useRadioShow } from './useRadioShow'

const BASE = 'http://localhost:8080'

describe('useRadioShow', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('uses the show(slug) query key', () => {
    expect(radioQueryKeys.show('drummer')).toEqual(['radio-shows', 'drummer'])
  })

  it('fetches a single show by slug', async () => {
    const mockResponse = { id: 1, name: 'The Drummer Show', slug: 'drummer' }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useRadioShow('drummer'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(`${BASE}/radio-shows/drummer`, {
      method: 'GET',
    })
    expect(result.current.data).toEqual(mockResponse)
  })

  it('does not fetch when slug is empty', () => {
    const { result } = renderHook(() => useRadioShow(''), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('starts in a loading state before the request resolves', () => {
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))
    const { result } = renderHook(() => useRadioShow('drummer'), {
      wrapper: createWrapper(),
    })
    expect(result.current.isLoading).toBe(true)
  })

  it('surfaces a not-found error', async () => {
    const error = new Error('Show not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useRadioShow('nope'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Show not found')
  })
})
