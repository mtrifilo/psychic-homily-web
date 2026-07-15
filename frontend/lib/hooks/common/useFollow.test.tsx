import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapper, createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()
const mockInvalidateFollows = vi.fn()
const mockInvalidatePersonalCharts = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    FOLLOW: {
      ENTITY: (entityType: string, entityId: number) =>
        `/${entityType}/${entityId}/follow`,
      FOLLOWERS: (entityType: string, entityId: number) =>
        `/${entityType}/${entityId}/followers`,
      BATCH: '/follows/batch',
      MY_FOLLOWING: '/me/following',
      LIBRARY_FOLLOWING: '/me/library/following',
      LIBRARY_COUNTS: '/me/library/following/counts',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    follows: {
      entity: (
        entityType: string,
        entityId: number,
        userId?: string | number
      ) => ['follows', entityType, userId ?? null, entityId],
      batch: (
        entityType: string,
        entityIds: number[],
        userId?: string | number
      ) => ['follows', 'batch', entityType, userId ?? null, ...entityIds],
      batchPrefix: (entityType: string, userId?: string | number) => [
        'follows',
        'batch',
        entityType,
        userId ?? null,
      ],
      myFollowing: (params?: Record<string, unknown>) => [
        'follows',
        'my-following',
        params,
      ],
      libraryCounts: (userId?: string | number) => [
        'follows',
        'library',
        'counts',
        userId ?? null,
      ],
      libraryFollowing: (entityType: string, userId?: string | number) => [
        'follows',
        'library',
        'following',
        userId ?? null,
        entityType,
      ],
    },
  },
  createInvalidateQueries: () => ({
    follows: mockInvalidateFollows,
    personalCharts: mockInvalidatePersonalCharts,
  }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: true, user: { id: 1 } }),
}))

import {
  useFollowStatus,
  useBatchFollowStatus,
  useFollow,
  useUnfollow,
  useMyFollowing,
  useAllMyFollowing,
  useLibraryFollowing,
  useLibraryFollowingCounts,
} from './useFollow'

type CachedFollow = { follower_count: number; is_following: boolean }

function createDeferred<T = void>(): {
  promise: Promise<T>
  resolve: (value: T) => void
} {
  let resolve: (value: T) => void = () => {}
  const promise = new Promise<T>(res => {
    resolve = res
  })
  return { promise, resolve }
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
    expect(mockApiRequest).toHaveBeenCalledWith('/artists/1/followers', {
      method: 'GET',
    })
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

  it('does not fetch when pre-fetched status disables the query', () => {
    const { result } = renderHook(() => useFollowStatus('artists', 1, false), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
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
    const { result } = renderHook(() => useBatchFollowStatus('artists', []), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useFollow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateFollows.mockReset()
    mockInvalidatePersonalCharts.mockReset()
  })

  it('follows an entity with POST', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, message: 'Followed' })

    const { result } = renderHook(() => useFollow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ entityType: 'artists', entityId: 1 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/artists/1/follow',
      expect.objectContaining({ method: 'POST' })
    )
    expect(mockInvalidatePersonalCharts).toHaveBeenCalled()
  })

  it('refreshes first activity after following a non-artist', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, message: 'Followed' })

    const { result } = renderHook(() => useFollow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ entityType: 'venues', entityId: 1 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidatePersonalCharts).toHaveBeenCalled()
  })

  it('does not hold a successful follow open on personal-stats refresh', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, message: 'Followed' })
    mockInvalidatePersonalCharts.mockReturnValueOnce(new Promise(() => {}))

    const { result } = renderHook(() => useFollow(), {
      wrapper: createWrapper(),
    })

    await expect(
      result.current.mutateAsync({ entityType: 'artists', entityId: 1 })
    ).resolves.toEqual({ success: true, message: 'Followed' })
  })

  it('performs optimistic update on follow', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: Infinity },
        mutations: { retry: false },
      },
    })

    queryClient.setQueryData(['follows', 'artists', 1, 1], {
      follower_count: 10,
      is_following: false,
    })
    const batchKey = ['follows', 'batch', 'artists', 1, 1, 2]
    queryClient.setQueryData(batchKey, {
      '1': { follower_count: 10, is_following: false },
      '2': { follower_count: 4, is_following: false },
    })
    const countsKey = ['follows', 'library', 'counts', 1]
    queryClient.setQueryData(countsKey, {
      artists: 4,
      venues: 2,
      scenes: 1,
      labels: 0,
      festivals: 0,
    })
    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useFollow(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({ entityType: 'artists', entityId: 1 })
    })

    const cached = queryClient.getQueryData<{
      follower_count: number
      is_following: boolean
    }>(['follows', 'artists', 1, 1])

    expect(cached?.follower_count).toBe(11)
    expect(cached?.is_following).toBe(true)
    expect(
      queryClient.getQueryData<
        Record<string, { follower_count: number; is_following: boolean }>
      >(batchKey)?.['1']
    ).toEqual({ follower_count: 11, is_following: true })
    expect(
      queryClient.getQueryData<{ artists: number }>(countsKey)?.artists
    ).toBe(5)
  })

  it('awaits cancelQueries before applying the optimistic update (regression)', async () => {
    // Regression for PSY-727: onMutate must `await queryClient.cancelQueries(...)`
    // before snapshotting/optimistically updating. Without the await, a concurrent
    // in-flight follow-status fetch can resolve and overwrite the optimistic
    // value, causing the user's click to "snap back".
    //
    // We assert ordering by making cancelQueries return a Promise we control:
    // - Trigger mutate(); while cancelQueries is unresolved, the cache must NOT
    //   yet hold the optimistic value (proves onMutate is waiting on the await).
    // - Resolve cancelQueries; the optimistic value must then appear in the
    //   cache (proves the optimistic update runs after the await resolves).
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: Infinity },
        mutations: { retry: false },
      },
    })

    queryClient.setQueryData(['follows', 'artists', 1, 1], {
      follower_count: 10,
      is_following: false,
    })
    const batchKey = ['follows', 'batch', 'artists', 1, 1, 2]
    queryClient.setQueryData(batchKey, {
      '1': { follower_count: 10, is_following: true },
      '2': { follower_count: 4, is_following: false },
    })
    const countsKey = ['follows', 'library', 'counts', 1]
    queryClient.setQueryData(countsKey, {
      artists: 4,
      venues: 2,
      scenes: 1,
      labels: 0,
      festivals: 0,
    })

    const cancelGate = createDeferred<void>()
    const cancelSpy = vi
      .spyOn(queryClient, 'cancelQueries')
      .mockImplementation(() => cancelGate.promise)

    mockApiRequest.mockResolvedValueOnce({ success: true, message: 'Followed' })

    const { result } = renderHook(() => useFollow(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    // Fire the mutation but don't await it -- onMutate should be parked on
    // the unresolved cancelQueries Promise.
    let mutatePromise: Promise<unknown>
    act(() => {
      mutatePromise = result.current.mutateAsync({
        entityType: 'artists',
        entityId: 1,
      })
    })

    // Yield microtasks; cache must NOT yet show the optimistic update.
    await Promise.resolve()
    await Promise.resolve()
    expect(
      queryClient.getQueryData<CachedFollow>(['follows', 'artists', 1, 1])
    ).toEqual({ follower_count: 10, is_following: false })
    expect(cancelSpy).toHaveBeenCalledWith({
      queryKey: ['follows', 'artists', 1, 1],
    })

    // Release the cancellation; optimistic update must now run.
    await act(async () => {
      cancelGate.resolve()
      await mutatePromise
    })

    expect(
      queryClient.getQueryData<CachedFollow>(['follows', 'artists', 1, 1])
    ).toEqual({ follower_count: 11, is_following: true })
  })
})

describe('useUnfollow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateFollows.mockReset()
    mockInvalidatePersonalCharts.mockReset()
  })

  it('unfollows an entity with DELETE', async () => {
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

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/artists/1/follow',
      expect.objectContaining({ method: 'DELETE' })
    )
    expect(mockInvalidatePersonalCharts).toHaveBeenCalled()
  })

  it('refreshes first activity after unfollowing a non-artist', async () => {
    mockApiRequest.mockResolvedValueOnce({
      success: true,
      message: 'Unfollowed',
    })

    const { result } = renderHook(() => useUnfollow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ entityType: 'venues', entityId: 1 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidatePersonalCharts).toHaveBeenCalled()
  })

  it('performs optimistic update on unfollow', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: Infinity },
        mutations: { retry: false },
      },
    })

    queryClient.setQueryData(['follows', 'artists', 1, 1], {
      follower_count: 10,
      is_following: true,
    })
    const batchKey = ['follows', 'batch', 'artists', 1, 1, 2]
    queryClient.setQueryData(batchKey, {
      '1': { follower_count: 10, is_following: true },
      '2': { follower_count: 4, is_following: false },
    })
    const countsKey = ['follows', 'library', 'counts', 1]
    queryClient.setQueryData(countsKey, {
      artists: 4,
      venues: 2,
      scenes: 1,
      labels: 0,
      festivals: 0,
    })
    const allFollowingKey = [
      'follows',
      'my-following',
      { type: 'all', scope: 'all', userId: 1 },
    ]
    queryClient.setQueryData(allFollowingKey, {
      following: [
        { entity_type: 'artist', entity_id: 1, name: 'Remove me', slug: 'a' },
        { entity_type: 'venue', entity_id: 1, name: 'Keep me', slug: 'v' },
      ],
      total: 2,
      limit: 2,
      offset: 0,
    })

    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useUnfollow(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({ entityType: 'artists', entityId: 1 })
    })

    const cached = queryClient.getQueryData<{
      follower_count: number
      is_following: boolean
    }>(['follows', 'artists', 1, 1])

    expect(cached?.follower_count).toBe(9)
    expect(cached?.is_following).toBe(false)
    expect(
      queryClient.getQueryData<
        Record<string, { follower_count: number; is_following: boolean }>
      >(batchKey)?.['1']
    ).toEqual({ follower_count: 9, is_following: false })
    expect(queryClient.getQueryData(countsKey)).toEqual({
      artists: 3,
      venues: 2,
      scenes: 1,
      labels: 0,
      festivals: 0,
    })
    expect(
      queryClient
        .getQueryData<{ following: Array<{ name: string }> }>(allFollowingKey)
        ?.following.map(entity => entity.name)
    ).toEqual(['Keep me'])
  })

  it('awaits cancelQueries before applying the optimistic update (regression)', async () => {
    // Parallel to the useFollow case -- unfollow's onMutate must also await
    // cancelQueries so a stale in-flight fetch can't clobber the optimistic
    // unfollow.
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: Infinity },
        mutations: { retry: false },
      },
    })

    queryClient.setQueryData(['follows', 'artists', 1, 1], {
      follower_count: 10,
      is_following: true,
    })

    const cancelGate = createDeferred<void>()
    const cancelSpy = vi
      .spyOn(queryClient, 'cancelQueries')
      .mockImplementation(() => cancelGate.promise)

    mockApiRequest.mockResolvedValueOnce({
      success: true,
      message: 'Unfollowed',
    })

    const { result } = renderHook(() => useUnfollow(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    let mutatePromise: Promise<unknown>
    act(() => {
      mutatePromise = result.current.mutateAsync({
        entityType: 'artists',
        entityId: 1,
      })
    })

    await Promise.resolve()
    await Promise.resolve()
    expect(
      queryClient.getQueryData<CachedFollow>(['follows', 'artists', 1, 1])
    ).toEqual({ follower_count: 10, is_following: true })
    expect(cancelSpy).toHaveBeenCalledWith({
      queryKey: ['follows', 'artists', 1, 1],
    })

    await act(async () => {
      cancelGate.resolve()
      await mutatePromise
    })

    expect(
      queryClient.getQueryData<CachedFollow>(['follows', 'artists', 1, 1])
    ).toEqual({ follower_count: 9, is_following: false })
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
    expect(mockInvalidatePersonalCharts).toHaveBeenCalled()
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

  it('scopes private following caches to the authenticated user', async () => {
    mockApiRequest.mockResolvedValueOnce({ following: [], total: 0 })
    const queryClient = new QueryClient()

    const { result } = renderHook(() => useMyFollowing({ type: 'artist' }), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(queryClient.getQueryCache().getAll()[0].queryKey).toEqual([
      'follows',
      'my-following',
      { type: 'artist', limit: 20, offset: 0, userId: 1 },
    ])
  })

  it('fetches every API page for a complete management list', async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) => ({
      entity_type: 'artist',
      entity_id: index + 1,
      name: `Artist ${index + 1}`,
      slug: `artist-${index + 1}`,
      followed_at: '2026-07-01T00:00:00Z',
    }))
    mockApiRequest
      .mockResolvedValueOnce({
        following: firstPage,
        total: 101,
        limit: 100,
        offset: 0,
      })
      .mockResolvedValueOnce({
        following: [
          {
            entity_type: 'artist',
            entity_id: 101,
            name: 'Artist 101',
            slug: 'artist-101',
            followed_at: '2026-07-01T00:00:00Z',
          },
        ],
        total: 101,
        limit: 100,
        offset: 100,
      })

    const queryClient = new QueryClient()
    const { result } = renderHook(() => useAllMyFollowing('artist'), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(queryClient.getQueryCache().getAll()[0].queryKey).toEqual([
      'follows',
      'my-following',
      { type: 'artist', scope: 'all', userId: 1 },
    ])
    expect(result.current.data?.following).toHaveLength(101)
    expect(mockApiRequest).toHaveBeenCalledTimes(2)
    expect(mockApiRequest.mock.calls[0][0]).toContain('offset=0')
    expect(mockApiRequest.mock.calls[1][0]).toContain('offset=100')
  })
})

describe('Library following read model', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches all tab counts in one request', async () => {
    mockApiRequest.mockResolvedValueOnce({
      artists: 4,
      venues: 2,
      scenes: 1,
      labels: 0,
      festivals: 3,
    })

    const { result } = renderHook(() => useLibraryFollowingCounts(), {
      wrapper: createWrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledTimes(1)
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/me/library/following/counts',
      { method: 'GET' }
    )
  })

  it('requests bounded alphabetical pages progressively', async () => {
    mockApiRequest
      .mockResolvedValueOnce({
        following: [{ entity_id: 1, name: 'Alpha' }],
        total: 2,
        limit: 50,
        offset: 0,
      })
      .mockResolvedValueOnce({
        following: [{ entity_id: 2, name: 'Beta' }],
        total: 2,
        limit: 50,
        offset: 1,
      })

    const { result } = renderHook(() => useLibraryFollowing('artist'), {
      wrapper: createWrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.hasNextPage).toBe(true)

    await act(async () => {
      await result.current.fetchNextPage()
    })

    expect(mockApiRequest).toHaveBeenCalledTimes(2)
    expect(mockApiRequest.mock.calls[0][0]).toContain(
      '/me/library/following?type=artist&limit=50&offset=0'
    )
    expect(mockApiRequest.mock.calls[1][0]).toContain('offset=1')
    await waitFor(() => expect(result.current.data?.pages).toHaveLength(2))
  })

  it('stops pagination when a concurrent change produces an empty page', async () => {
    mockApiRequest.mockResolvedValueOnce({
      following: [],
      total: 1,
      limit: 50,
      offset: 0,
    })

    const { result } = renderHook(() => useLibraryFollowing('artist'), {
      wrapper: createWrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.hasNextPage).toBe(false)
  })
})
