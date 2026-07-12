import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapper, createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()
const mockInvalidateFollows = vi.fn()

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
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    follows: {
      entity: (entityType: string, entityId: number) => [
        'follows',
        entityType,
        entityId,
      ],
      batch: (entityType: string, entityIds: number[]) => [
        'follows',
        'batch',
        entityType,
        ...entityIds,
      ],
      myFollowing: (params?: Record<string, unknown>) => [
        'follows',
        'my-following',
        params,
      ],
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
  })

  it('performs optimistic update on follow', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: Infinity },
        mutations: { retry: false },
      },
    })

    queryClient.setQueryData(['follows', 'artists', 1], {
      follower_count: 10,
      is_following: false,
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
    }>(['follows', 'artists', 1])

    expect(cached?.follower_count).toBe(11)
    expect(cached?.is_following).toBe(true)
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

    queryClient.setQueryData(['follows', 'artists', 1], {
      follower_count: 10,
      is_following: false,
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
      queryClient.getQueryData<CachedFollow>(['follows', 'artists', 1])
    ).toEqual({ follower_count: 10, is_following: false })
    expect(cancelSpy).toHaveBeenCalledWith({
      queryKey: ['follows', 'artists', 1],
    })

    // Release the cancellation; optimistic update must now run.
    await act(async () => {
      cancelGate.resolve()
      await mutatePromise
    })

    expect(
      queryClient.getQueryData<CachedFollow>(['follows', 'artists', 1])
    ).toEqual({ follower_count: 11, is_following: true })
  })
})

describe('useUnfollow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateFollows.mockReset()
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
  })

  it('performs optimistic update on unfollow', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: Infinity },
        mutations: { retry: false },
      },
    })

    queryClient.setQueryData(['follows', 'artists', 1], {
      follower_count: 10,
      is_following: true,
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
    }>(['follows', 'artists', 1])

    expect(cached?.follower_count).toBe(9)
    expect(cached?.is_following).toBe(false)
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

    queryClient.setQueryData(['follows', 'artists', 1], {
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
      queryClient.getQueryData<CachedFollow>(['follows', 'artists', 1])
    ).toEqual({ follower_count: 10, is_following: true })
    expect(cancelSpy).toHaveBeenCalledWith({
      queryKey: ['follows', 'artists', 1],
    })

    await act(async () => {
      cancelGate.resolve()
      await mutatePromise
    })

    expect(
      queryClient.getQueryData<CachedFollow>(['follows', 'artists', 1])
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
