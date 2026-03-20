import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      STATS: '/admin/stats',
      ACTIVITY: '/admin/activity',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    admin: {
      stats: ['admin', 'stats'],
      activity: ['admin', 'activity'],
    },
  },
}))

import { useAdminStats, useAdminActivity } from './useAdminStats'

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
    },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }
}

describe('useAdminStats', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches admin dashboard stats', async () => {
    const mockStats = {
      total_shows: 100,
      total_artists: 50,
      total_venues: 20,
      pending_shows: 5,
    }
    mockApiRequest.mockResolvedValueOnce(mockStats)

    const { result } = renderHook(() => useAdminStats(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/admin/stats', { method: 'GET' })
    expect(result.current.data?.total_shows).toBe(100)
  })

  it('handles API errors', async () => {
    const error = new Error('Forbidden')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useAdminStats(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})

describe('useAdminActivity', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches admin activity feed', async () => {
    const mockActivity = {
      events: [
        { id: 1, event_type: 'show_approved', created_at: '2025-03-17T12:00:00Z' },
        { id: 2, event_type: 'artist_updated', created_at: '2025-03-17T11:00:00Z' },
      ],
    }
    mockApiRequest.mockResolvedValueOnce(mockActivity)

    const { result } = renderHook(() => useAdminActivity(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/admin/activity', { method: 'GET' })
    expect(result.current.data?.events).toHaveLength(2)
  })

  it('handles empty activity feed', async () => {
    mockApiRequest.mockResolvedValueOnce({ events: [] })

    const { result } = renderHook(() => useAdminActivity(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.events).toEqual([])
  })
})
