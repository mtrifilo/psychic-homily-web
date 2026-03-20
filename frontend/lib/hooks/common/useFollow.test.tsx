import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

/**
 * Create a test query client that retains cache data (gcTime > 0).
 * Needed for optimistic update tests where we seed cache data without an active observer.
 */
function createRetainingQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 5 * 60 * 1000, // 5 minutes
      },
      mutations: {
        retry: false,
      },
    },
  })
}

// Create mocks
const mockApiRequest = vi.fn()
const mockIsAuthenticated = vi.fn().mockReturnValue(true)

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    FOLLOW: {
      ENTITY: (entityType: string, entityId: number) =>
        `/api/${entityType}/${entityId}/follow`,
      FOLLOWERS: (entityType: string, entityId: number) =>
        `/api/${entityType}/${entityId}/followers`,
      BATCH: '/api/follows/batch',
      MY_FOLLOWING: '/api/me/following',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    follows: {
      all: ['follows'] as const,
      entity: (entityType: string, entityId: number) =>
        ['follows', entityType, entityId] as const,
      batch: (entityType: string, entityIds: number[]) =>
        ['follows', 'batch', entityType, ...entityIds] as const,
      myFollowing: (params?: Record<string, unknown>) =>
        ['follows', 'my-following', params] as const,
    },
  },
  createInvalidateQueries: (queryClient: QueryClient) => ({
    follows: () => queryClient.invalidateQueries({ queryKey: ['follows'] }),
  }),
}))

// Mock AuthContext
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    isAuthenticated: mockIsAuthenticated(),
  }),
}))

// Import hooks after mocks are set up
import {
  useFollowStatus,
  useBatchFollowStatus,
  useFollow,
  useUnfollow,
  useMyFollowing,
} from './useFollow'

describe('useFollow hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockIsAuthenticated.mockReturnValue(true)
  })

  describe('useFollowStatus', () => {
    it('fetches follow status for an entity', async () => {
      const mockStatus = {
        entity_type: 'artists',
        entity_id: 1,
        follower_count: 42,
        is_following: true,
      }
      mockApiRequest.mockResolvedValueOnce(mockStatus)

      const { result } = renderHook(() => useFollowStatus('artists', 1), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/api/artists/1/followers',
        { method: 'GET' }
      )
      expect(result.current.data?.follower_count).toBe(42)
      expect(result.current.data?.is_following).toBe(true)
    })

    it('does not fetch when entityId is 0', () => {
      const { result } = renderHook(() => useFollowStatus('artists', 0), {
        wrapper: createWrapper(),
      })

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when entityId is negative', () => {
      const { result } = renderHook(() => useFollowStatus('artists', -1), {
        wrapper: createWrapper(),
      })

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when entityType is empty', () => {
      const { result } = renderHook(() => useFollowStatus('', 1), {
        wrapper: createWrapper(),
      })

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useFollowStatus('artists', 1), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect(result.current.error).toBeDefined()
    })

    it('works with different entity types', async () => {
      const mockStatus = {
        entity_type: 'venues',
        entity_id: 5,
        follower_count: 10,
        is_following: false,
      }
      mockApiRequest.mockResolvedValueOnce(mockStatus)

      const { result } = renderHook(() => useFollowStatus('venues', 5), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/api/venues/5/followers',
        { method: 'GET' }
      )
      expect(result.current.data?.is_following).toBe(false)
    })
  })

  describe('useBatchFollowStatus', () => {
    it('fetches batch follow status for multiple entities', async () => {
      const mockResponse = {
        follows: {
          '1': { follower_count: 10, is_following: true },
          '2': { follower_count: 5, is_following: false },
          '3': { follower_count: 20, is_following: true },
        },
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useBatchFollowStatus('artists', [1, 2, 3]),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/api/follows/batch', {
        method: 'POST',
        body: JSON.stringify({
          entity_type: 'artists',
          entity_ids: [1, 2, 3],
        }),
      })
      expect(result.current.data?.['1'].follower_count).toBe(10)
      expect(result.current.data?.['2'].is_following).toBe(false)
    })

    it('does not fetch when entityIds is empty', () => {
      const { result } = renderHook(
        () => useBatchFollowStatus('artists', []),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when entityType is empty', () => {
      const { result } = renderHook(
        () => useBatchFollowStatus('', [1, 2]),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () => useBatchFollowStatus('artists', [1, 2]),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useFollow', () => {
    it('sends POST to follow an entity', async () => {
      mockApiRequest.mockResolvedValueOnce({ success: true, message: 'Followed' })

      const { result } = renderHook(() => useFollow(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ entityType: 'artists', entityId: 1 })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/api/artists/1/follow', {
        method: 'POST',
      })
    })

    it('optimistically updates follow status', async () => {
      const queryClient = createRetainingQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      // Seed the cache with initial follow status
      queryClient.setQueryData(['follows', 'artists', 1], {
        entity_type: 'artists',
        entity_id: 1,
        follower_count: 10,
        is_following: false,
      })

      // Use a deferred promise so the mutation stays in-flight while we check
      let resolveApi!: (value: unknown) => void
      mockApiRequest.mockImplementation(
        () => new Promise((resolve) => { resolveApi = resolve })
      )

      const { result } = renderHook(() => useFollow(), { wrapper })

      act(() => {
        result.current.mutate({ entityType: 'artists', entityId: 1 })
      })

      // Check optimistic update was applied while mutation is still pending
      await waitFor(() => {
        const data = queryClient.getQueryData(['follows', 'artists', 1]) as {
          follower_count: number
          is_following: boolean
        }
        expect(data).toBeDefined()
        expect(data.follower_count).toBe(11)
        expect(data.is_following).toBe(true)
      })

      // Resolve the API call to clean up
      await act(async () => {
        resolveApi({ success: true, message: 'Followed' })
      })
    })

    it('rolls back optimistic update on error', async () => {
      const queryClient = createRetainingQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      // Seed cache
      const originalData = {
        entity_type: 'artists',
        entity_id: 1,
        follower_count: 10,
        is_following: false,
      }
      queryClient.setQueryData(['follows', 'artists', 1], originalData)

      // First call rejects (the mutation), second call returns original data (the refetch from onSettled)
      mockApiRequest
        .mockRejectedValueOnce(new Error('Network error'))
        .mockResolvedValueOnce(originalData)

      const { result } = renderHook(() => useFollow(), { wrapper })

      await act(async () => {
        result.current.mutate({ entityType: 'artists', entityId: 1 })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      // Should have rolled back via onError
      const data = queryClient.getQueryData(['follows', 'artists', 1]) as {
        follower_count: number
        is_following: boolean
      }
      expect(data.follower_count).toBe(10)
      expect(data.is_following).toBe(false)
    })

    it('handles mutation error gracefully', async () => {
      const error = new Error('Unauthorized')
      Object.assign(error, { status: 401 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useFollow(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ entityType: 'artists', entityId: 1 })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).message).toBe('Unauthorized')
    })
  })

  describe('useUnfollow', () => {
    it('sends DELETE to unfollow an entity', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Unfollowed',
      })

      const { result } = renderHook(() => useUnfollow(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ entityType: 'artists', entityId: 1 })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/api/artists/1/follow', {
        method: 'DELETE',
      })
    })

    it('optimistically updates unfollow status', async () => {
      const queryClient = createRetainingQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      // Seed the cache with following status
      queryClient.setQueryData(['follows', 'artists', 1], {
        entity_type: 'artists',
        entity_id: 1,
        follower_count: 10,
        is_following: true,
      })

      // Use a deferred promise so the mutation stays in-flight
      let resolveApi!: (value: unknown) => void
      mockApiRequest.mockImplementation(
        () => new Promise((resolve) => { resolveApi = resolve })
      )

      const { result } = renderHook(() => useUnfollow(), { wrapper })

      act(() => {
        result.current.mutate({ entityType: 'artists', entityId: 1 })
      })

      // Check optimistic update was applied while mutation is still pending
      await waitFor(() => {
        const data = queryClient.getQueryData(['follows', 'artists', 1]) as {
          follower_count: number
          is_following: boolean
        }
        expect(data).toBeDefined()
        expect(data.follower_count).toBe(9)
        expect(data.is_following).toBe(false)
      })

      // Resolve to clean up
      await act(async () => {
        resolveApi({ success: true, message: 'Unfollowed' })
      })
    })

    it('does not decrement follower_count below zero', async () => {
      const queryClient = createRetainingQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      // Seed cache with follower_count of 0
      queryClient.setQueryData(['follows', 'artists', 1], {
        entity_type: 'artists',
        entity_id: 1,
        follower_count: 0,
        is_following: true,
      })

      // Use a deferred promise so the mutation stays in-flight
      let resolveApi!: (value: unknown) => void
      mockApiRequest.mockImplementation(
        () => new Promise((resolve) => { resolveApi = resolve })
      )

      const { result } = renderHook(() => useUnfollow(), { wrapper })

      act(() => {
        result.current.mutate({ entityType: 'artists', entityId: 1 })
      })

      await waitFor(() => {
        const data = queryClient.getQueryData(['follows', 'artists', 1]) as {
          follower_count: number
          is_following: boolean
        }
        expect(data).toBeDefined()
        expect(data.follower_count).toBe(0)
        expect(data.is_following).toBe(false)
      })

      // Resolve to clean up
      await act(async () => {
        resolveApi({ success: true, message: 'Unfollowed' })
      })
    })

    it('rolls back optimistic update on error', async () => {
      const queryClient = createRetainingQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      const originalData = {
        entity_type: 'artists',
        entity_id: 1,
        follower_count: 10,
        is_following: true,
      }
      queryClient.setQueryData(['follows', 'artists', 1], originalData)

      // First call rejects (the mutation), second call returns original data (the refetch from onSettled)
      mockApiRequest
        .mockRejectedValueOnce(new Error('Network error'))
        .mockResolvedValueOnce(originalData)

      const { result } = renderHook(() => useUnfollow(), { wrapper })

      await act(async () => {
        result.current.mutate({ entityType: 'artists', entityId: 1 })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      const data = queryClient.getQueryData(['follows', 'artists', 1]) as {
        follower_count: number
        is_following: boolean
      }
      expect(data.follower_count).toBe(10)
      expect(data.is_following).toBe(true)
    })
  })

  describe('useMyFollowing', () => {
    it('fetches the authenticated user following list', async () => {
      const mockResponse = {
        following: [
          {
            entity_type: 'artists',
            entity_id: 1,
            name: 'Test Artist',
            slug: 'test-artist',
            followed_at: '2026-01-01T00:00:00Z',
          },
        ],
        total: 1,
        limit: 20,
        offset: 0,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useMyFollowing(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.following).toHaveLength(1)
      expect(result.current.data?.following[0].name).toBe('Test Artist')
    })

    it('does not fetch when user is not authenticated', () => {
      mockIsAuthenticated.mockReturnValue(false)

      const { result } = renderHook(() => useMyFollowing(), {
        wrapper: createWrapper(),
      })

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('includes type filter in query params', async () => {
      mockApiRequest.mockResolvedValueOnce({
        following: [],
        total: 0,
        limit: 20,
        offset: 0,
      })

      const { result } = renderHook(
        () => useMyFollowing({ type: 'artists' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0] as string
      expect(calledUrl).toContain('type=artists')
    })

    it('does not include type param when type is all', async () => {
      mockApiRequest.mockResolvedValueOnce({
        following: [],
        total: 0,
        limit: 20,
        offset: 0,
      })

      const { result } = renderHook(
        () => useMyFollowing({ type: 'all' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0] as string
      expect(calledUrl).not.toContain('type=')
    })

    it('includes limit and offset in query params', async () => {
      mockApiRequest.mockResolvedValueOnce({
        following: [],
        total: 0,
        limit: 10,
        offset: 20,
      })

      const { result } = renderHook(
        () => useMyFollowing({ limit: 10, offset: 20 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0] as string
      expect(calledUrl).toContain('limit=10')
      expect(calledUrl).toContain('offset=20')
    })

    it('uses default limit and offset', async () => {
      mockApiRequest.mockResolvedValueOnce({
        following: [],
        total: 0,
        limit: 20,
        offset: 0,
      })

      const { result } = renderHook(() => useMyFollowing(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0] as string
      expect(calledUrl).toContain('limit=20')
      expect(calledUrl).toContain('offset=0')
    })

    it('handles API errors', async () => {
      const error = new Error('Unauthorized')
      Object.assign(error, { status: 401 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useMyFollowing(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })
})
