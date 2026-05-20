import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { radioQueryKeys } from '../api'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useRadioEpisodes } from './useRadioEpisodes'

const BASE = 'http://localhost:8080'

describe('useRadioEpisodes', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('carries limit/offset in the episodes() query key', () => {
    expect(radioQueryKeys.episodes('drummer', { limit: 20, offset: 0 })).toEqual([
      'radio-shows',
      'drummer',
      'episodes',
      { limit: 20, offset: 0 },
    ])
  })

  it('fetches episodes with the default limit query param', async () => {
    const mockResponse = { episodes: [{ id: 1, air_date: '2026-05-01' }], total: 1 }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useRadioEpisodes({ showSlug: 'drummer' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Default limit=20, offset=0 → offset omitted (falsy), limit kept.
    expect(mockApiRequest).toHaveBeenCalledWith(
      `${BASE}/radio-shows/drummer/episodes?limit=20`,
      { method: 'GET' }
    )
    expect(result.current.data).toEqual(mockResponse)
  })

  it('includes both limit and offset when paginating', async () => {
    mockApiRequest.mockResolvedValueOnce({ episodes: [], total: 0 })

    const { result } = renderHook(
      () => useRadioEpisodes({ showSlug: 'drummer', limit: 10, offset: 20 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('limit=10')
    expect(calledUrl).toContain('offset=20')
  })

  it('does not fetch when showSlug is empty', () => {
    const { result } = renderHook(() => useRadioEpisodes({ showSlug: '' }), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when explicitly disabled', () => {
    const { result } = renderHook(
      () => useRadioEpisodes({ showSlug: 'drummer', enabled: false }),
      { wrapper: createWrapper() }
    )
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('starts in a loading state before the request resolves', () => {
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))
    const { result } = renderHook(() => useRadioEpisodes({ showSlug: 'drummer' }), {
      wrapper: createWrapper(),
    })
    expect(result.current.isLoading).toBe(true)
  })

  it('surfaces API errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useRadioEpisodes({ showSlug: 'drummer' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Server error')
  })
})
