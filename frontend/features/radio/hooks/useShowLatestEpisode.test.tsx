import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useShowLatestEpisode } from './useShowLatestEpisode'

const BASE = 'http://localhost:8080'

describe('useShowLatestEpisode', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('chains the episodes list (newest air_date) into the by-date episode detail', async () => {
    mockApiRequest.mockImplementation((url: string) => {
      if (url.includes('/episodes?') || url.endsWith('/episodes')) {
        return Promise.resolve({ episodes: [{ id: 9, air_date: '2026-06-04' }], total: 30 })
      }
      if (url.includes('/episodes/2026-06-04')) {
        return Promise.resolve({ id: 9, air_date: '2026-06-04', plays: [{ id: 1 }] })
      }
      throw new Error(`unexpected url: ${url}`)
    })

    const { result } = renderHook(() => useShowLatestEpisode('variety-mix'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.episode).toBeDefined())
    expect(result.current.episode?.air_date).toBe('2026-06-04')
    expect(result.current.hasEpisodes).toBe(true)
    // requests only the newest episode from the list endpoint
    expect(mockApiRequest).toHaveBeenCalledWith(
      `${BASE}/radio-shows/variety-mix/episodes?limit=1`,
      { method: 'GET' }
    )
    expect(mockApiRequest).toHaveBeenCalledWith(
      `${BASE}/radio-shows/variety-mix/episodes/2026-06-04`,
      { method: 'GET' }
    )
  })

  it('resolves to no episode (not loading) for a show with zero episodes', async () => {
    mockApiRequest.mockResolvedValueOnce({ episodes: [], total: 0 })

    const { result } = renderHook(() => useShowLatestEpisode('empty-show'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.episode).toBeUndefined()
    expect(result.current.hasEpisodes).toBe(false)
    // never reaches the by-date detail call
    expect(mockApiRequest).toHaveBeenCalledTimes(1)
  })

  it('does not fetch when the show slug is undefined', () => {
    const { result } = renderHook(() => useShowLatestEpisode(undefined), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.episode).toBeUndefined()
  })
})
