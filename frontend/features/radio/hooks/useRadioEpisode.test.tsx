import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { radioQueryKeys } from '../api'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useRadioEpisode } from './useRadioEpisode'

const BASE = 'http://localhost:8080'

describe('useRadioEpisode', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('uses the episode(slug, date) query key', () => {
    expect(radioQueryKeys.episode('drummer', '2026-05-01')).toEqual([
      'radio-shows',
      'drummer',
      'episodes',
      '2026-05-01',
    ])
  })

  it('fetches a single episode by show slug + date', async () => {
    const mockResponse = { id: 1, show_slug: 'drummer', air_date: '2026-05-01' }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useRadioEpisode('drummer', '2026-05-01'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      `${BASE}/radio-shows/drummer/episodes/2026-05-01`,
      { method: 'GET' }
    )
    expect(result.current.data).toEqual(mockResponse)
  })

  it('does not fetch when slug is empty', () => {
    const { result } = renderHook(() => useRadioEpisode('', '2026-05-01'), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when date is empty', () => {
    const { result } = renderHook(() => useRadioEpisode('drummer', ''), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('starts in a loading state before the request resolves', () => {
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))
    const { result } = renderHook(() => useRadioEpisode('drummer', '2026-05-01'), {
      wrapper: createWrapper(),
    })
    expect(result.current.isLoading).toBe(true)
  })

  it('polls a live episode every ~60s (PSY-1511 live ledger)', async () => {
    vi.useFakeTimers()
    try {
      const live = {
        id: 1,
        show_slug: 'drummer',
        air_date: '2026-05-01',
        starts_at: new Date(Date.now() - 30 * 60 * 1000).toISOString(),
        ends_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
      }
      mockApiRequest.mockResolvedValue(live)

      const { result } = renderHook(() => useRadioEpisode('drummer', '2026-05-01'), {
        wrapper: createWrapper(),
      })
      await vi.advanceTimersByTimeAsync(0)
      expect(result.current.isSuccess).toBe(true)
      expect(mockApiRequest).toHaveBeenCalledTimes(1)

      await vi.advanceTimersByTimeAsync(61 * 1000)
      expect(mockApiRequest).toHaveBeenCalledTimes(2)
    } finally {
      vi.useRealTimers()
    }
  })

  it('stops polling after a failed refetch (error-guarded wiring, PSY-1136 class)', async () => {
    vi.useFakeTimers()
    try {
      const live = {
        id: 1,
        show_slug: 'drummer',
        air_date: '2026-05-01',
        starts_at: new Date(Date.now() - 30 * 60 * 1000).toISOString(),
        ends_at: new Date(Date.now() + 3 * 60 * 60 * 1000).toISOString(),
      }
      mockApiRequest.mockResolvedValueOnce(live)

      renderHook(() => useRadioEpisode('drummer', '2026-05-01'), {
        wrapper: createWrapper(),
      })
      await vi.advanceTimersByTimeAsync(0)
      expect(mockApiRequest).toHaveBeenCalledTimes(1)

      // Next poll fails; the interval must not keep firing afterwards.
      mockApiRequest.mockRejectedValue(new Error('boom'))
      await vi.advanceTimersByTimeAsync(61 * 1000)
      expect(mockApiRequest).toHaveBeenCalledTimes(2)

      await vi.advanceTimersByTimeAsync(3 * 61 * 1000)
      expect(mockApiRequest).toHaveBeenCalledTimes(2)
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not poll an aired episode', async () => {
    vi.useFakeTimers()
    try {
      const aired = {
        id: 1,
        show_slug: 'drummer',
        air_date: '2026-05-01',
        starts_at: new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString(),
        ends_at: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
      }
      mockApiRequest.mockResolvedValue(aired)

      const { result } = renderHook(() => useRadioEpisode('drummer', '2026-05-01'), {
        wrapper: createWrapper(),
      })
      await vi.advanceTimersByTimeAsync(0)
      expect(result.current.isSuccess).toBe(true)

      await vi.advanceTimersByTimeAsync(3 * 61 * 1000)
      expect(mockApiRequest).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('surfaces a not-found error', async () => {
    const error = new Error('Episode not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useRadioEpisode('drummer', '2026-01-01'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Episode not found')
  })
})
