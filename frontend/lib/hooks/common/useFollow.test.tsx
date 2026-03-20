import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockApiRequest = vi.fn()
const mockInvalidateFollows = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    FOLLOW: {
      ENTITY: (entityType: string, entityId: number) => `/${entityType}/${entityId}/follow`,
      FOLLOWERS: (entityType: string, entityId: number) => `/${entityType}/${entityId}/followers`,
      BATCH: '/follows/batch',
      MY_FOLLOWING: '/me/following',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    follows: {
      entity: (entityType: string, entityId: number) => ['follows', entityType, entityId],
      batch: (entityType: string, entityIds: number[]) => ['follows', 'batch', entityType, ...entityIds],
      myFollowing: (params?: Record<string, unknown>) => ['follows', 'my-following', params],
    },
  },
  createInvalidateQueries: () => ({
    follows: mockInvalidateFollows,
  }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: true }),
}))

import {
  useFollowStatus,
  useBatchFollowStatus,
  useFollow,
  useUnfollow,
  useMyFollowing,
} from './useFollow'

function createWrapper(queryClient?: QueryClient) {
  const qc =
    queryClient ??
    new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 0 },
        mutations: { retry: false },
      },
    })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

describe('useFollowStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches follow status for an entity', async () => {
    mockApiRequest.mockResolvedValueOnce({
      follower_count: 42,
      is_following: true,
    })

    const { result } = renderHook(() => useFollowStatus('artists', 1), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/artists/1/followers', { method: 'GET' })
    expect(result.current.data?.follower_count).toBe(42)
    expect(result.current.data?.is_following).toBe(true)
  })

  it('does not fetch when entityId is 0', () => {
    const { result } = renderHook(() => useFollowStatus('artists', 0), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when entityType is empty', () => {
    const { result } = renderHook(() => useFollowStatus('', 1), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useBatchFollowStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches batch follow status via POST', async () => {
    mockApiRequest.mockResolvedValueOnce({
      follows: {
        '1': { follower_count: 10, is_following: true },
        '2': { follower_count: 5, is_following: false },
      },
    })

    const { result } = renderHook(
      () => useBatchFollowStatus('artists', [1, 2]),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/follows/batch',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ entity_type: 'artists', entity_ids: [1, 2] }),
      })
    )
  })

  it('does not fetch when entityIds is empty', () => {
    const { result } = renderHook(
      () => useBatchFollowStatus('artists', []),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useFollow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateFollows.mockReset()
  })

  it('follows an entity with POST', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, message: 'Followed' })

    const { result } = renderHook(() => useFollow(), { wrapper: createWrapper() })

    await act(async () => {
      result.current.mutate({ entityType: 'artists', entityId: 1 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/artists/1/follow',
      expect.objectContaining({ method: 'POST' })
    )
  })

  it('performs optimistic update on follow', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
    })

    queryClient.setQueryData(['follows', 'artists', 1], {
      follower_count: 10,
      is_following: false,
    })

    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useFollow(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate({ entityType: 'artists', entityId: 1 })
    })

    const cached = queryClient.getQueryData<{
      follower_count: number
      is_following: boolean
    }>(['follows', 'artists', 1])

    expect(cached?.follower_count).toBe(11)
    expect(cached?.is_following).toBe(true)
  })
})

describe('useUnfollow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateFollows.mockReset()
  })

  it('unfollows an entity with DELETE', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, message: 'Unfollowed' })

    const { result } = renderHook(() => useUnfollow(), { wrapper: createWrapper() })

    await act(async () => {
      result.current.mutate({ entityType: 'artists', entityId: 1 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/artists/1/follow',
      expect.objectContaining({ method: 'DELETE' })
    )
  })

  it('performs optimistic update on unfollow', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
    })

    queryClient.setQueryData(['follows', 'artists', 1], {
      follower_count: 10,
      is_following: true,
    })

    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useUnfollow(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate({ entityType: 'artists', entityId: 1 })
    })

    const cached = queryClient.getQueryData<{
      follower_count: number
      is_following: boolean
    }>(['follows', 'artists', 1])

    expect(cached?.follower_count).toBe(9)
    expect(cached?.is_following).toBe(false)
  })

  it('handles unfollow errors', async () => {
    const error = new Error('Network error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useUnfollow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ entityType: 'artists', entityId: 1 })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeDefined()
  })
})

describe('useMyFollowing', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches user following list with defaults', async () => {
    mockApiRequest.mockResolvedValueOnce({ following: [], total: 0 })

    const { result } = renderHook(() => useMyFollowing(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/me/following')
    expect(url).toContain('limit=20')
    expect(url).toContain('offset=0')
    expect(url).not.toContain('type=')
  })

  it('includes type filter', async () => {
    mockApiRequest.mockResolvedValueOnce({ following: [], total: 0 })

    const { result } = renderHook(() => useMyFollowing({ type: 'artists' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest.mock.calls[0][0]).toContain('type=artists')
  })
})
