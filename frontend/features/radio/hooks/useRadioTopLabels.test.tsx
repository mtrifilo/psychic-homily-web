import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { radioQueryKeys } from '../api'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useRadioTopLabels } from './useRadioTopLabels'

describe('useRadioTopLabels', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('carries period/limit in the topLabels() query key', () => {
    expect(radioQueryKeys.topLabels('drummer', { period: 90, limit: 20 })).toEqual([
      'radio-shows',
      'drummer',
      'top-labels',
      { period: 90, limit: 20 },
    ])
  })

  it('fetches with default period + limit query params', async () => {
    const mockResponse = {
      labels: [{ label_name: 'Relapse', play_count: 6 }],
      count: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useRadioTopLabels({ showSlug: 'drummer' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('/radio-shows/drummer/top-labels?')
    expect(calledUrl).toContain('period=90')
    expect(calledUrl).toContain('limit=20')
    expect(result.current.data).toEqual(mockResponse)
  })

  it('honors a custom period and limit', async () => {
    mockApiRequest.mockResolvedValueOnce({ labels: [], count: 0 })

    const { result } = renderHook(
      () => useRadioTopLabels({ showSlug: 'drummer', period: 30, limit: 5 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('period=30')
    expect(calledUrl).toContain('limit=5')
  })

  it('does not fetch when showSlug is empty', () => {
    const { result } = renderHook(() => useRadioTopLabels({ showSlug: '' }), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when explicitly disabled', () => {
    const { result } = renderHook(
      () => useRadioTopLabels({ showSlug: 'drummer', enabled: false }),
      { wrapper: createWrapper() }
    )
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('starts in a loading state before the request resolves', () => {
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))
    const { result } = renderHook(() => useRadioTopLabels({ showSlug: 'drummer' }), {
      wrapper: createWrapper(),
    })
    expect(result.current.isLoading).toBe(true)
  })

  it('surfaces API errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useRadioTopLabels({ showSlug: 'drummer' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Server error')
  })
})
