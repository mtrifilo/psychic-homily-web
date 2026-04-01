import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    COMMUNITY: {
      LEADERBOARD: '/community/leaderboard',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    community: {
      leaderboard: (dimension: string, period: string, limit?: number) =>
        ['community', 'leaderboard', dimension, period, limit],
    },
  },
}))

import { useLeaderboard } from './useLeaderboard'

describe('useLeaderboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches leaderboard with default params', async () => {
    const mockData = {
      entries: [
        {
          rank: 1,
          user_id: 1,
          username: 'alice',
          user_tier: 'contributor',
          count: 42,
        },
      ],
      dimension: 'overall',
      period: 'all_time',
    }
    mockApiRequest.mockResolvedValueOnce(mockData)

    const { result } = renderHook(() => useLeaderboard(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/community/leaderboard?dimension=overall&period=all_time',
      { method: 'GET' },
    )
    expect(result.current.data?.entries).toHaveLength(1)
    expect(result.current.data?.entries[0].username).toBe('alice')
  })

  it('passes dimension and period params', async () => {
    mockApiRequest.mockResolvedValueOnce({
      entries: [],
      dimension: 'shows',
      period: 'week',
    })

    const { result } = renderHook(() => useLeaderboard('shows', 'week'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/community/leaderboard?dimension=shows&period=week',
      { method: 'GET' },
    )
  })

  it('passes limit param when provided', async () => {
    mockApiRequest.mockResolvedValueOnce({
      entries: [],
      dimension: 'overall',
      period: 'all_time',
    })

    const { result } = renderHook(() => useLeaderboard('overall', 'all_time', 10), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/community/leaderboard?dimension=overall&period=all_time&limit=10',
      { method: 'GET' },
    )
  })

  it('includes user_rank in response when available', async () => {
    const mockData = {
      entries: [
        {
          rank: 1,
          user_id: 2,
          username: 'top-user',
          user_tier: 'trusted_contributor',
          count: 100,
        },
      ],
      dimension: 'overall',
      period: 'all_time',
      user_rank: 5,
    }
    mockApiRequest.mockResolvedValueOnce(mockData)

    const { result } = renderHook(() => useLeaderboard(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.user_rank).toBe(5)
  })
})
