import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { radioQueryKeys } from '../api'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useReleaseRadioPlays } from './useReleaseRadioPlays'

const BASE = 'http://localhost:8080'

describe('useReleaseRadioPlays', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('uses the releasePlays(slug) query key', () => {
    expect(radioQueryKeys.releasePlays('an-unkindness')).toEqual([
      'releases',
      'an-unkindness',
      'radio-plays',
    ])
  })

  it('fetches radio plays for the release', async () => {
    const mockResponse = {
      stations: [{ station_id: 1, station_name: 'KEXP', play_count: 2 }],
      count: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useReleaseRadioPlays('an-unkindness'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      `${BASE}/releases/an-unkindness/radio-plays`,
      { method: 'GET' }
    )
    expect(result.current.data).toEqual(mockResponse)
  })

  it('does not fetch when slug is empty', () => {
    const { result } = renderHook(() => useReleaseRadioPlays(''), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when explicitly disabled', () => {
    const { result } = renderHook(() => useReleaseRadioPlays('an-unkindness', false), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('starts in a loading state before the request resolves', () => {
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))
    const { result } = renderHook(() => useReleaseRadioPlays('an-unkindness'), {
      wrapper: createWrapper(),
    })
    expect(result.current.isLoading).toBe(true)
  })

  it('surfaces API errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useReleaseRadioPlays('an-unkindness'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Server error')
  })
})
