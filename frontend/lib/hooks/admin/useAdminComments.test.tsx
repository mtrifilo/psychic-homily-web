import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import {
  createWrapper,
  createWrapperWithClient,
  createTestQueryClient,
} from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/features/comments/api', () => ({
  commentQueryKeys: {
    all: ['comments'],
    entity: (entityType: string, entityId: number) => ['comments', entityType, entityId],
    thread: (commentId: number) => ['comments', 'thread', commentId],
  },
}))

import {
  useAdminPendingComments,
  useAdminApproveComment,
  useAdminRejectComment,
  useAdminHideComment,
  useAdminRestoreComment,
  useAdminCommentEditHistory,
  adminCommentQueryKeys,
} from './useAdminComments'

describe('useAdminPendingComments', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches pending comments with default pagination', async () => {
    const mockResponse = {
      comments: [
        {
          id: 1,
          entity_type: 'artist',
          entity_id: 42,
          author_name: 'TestUser',
          body: 'Great artist!',
          body_html: '<p>Great artist!</p>',
          visibility: 'pending',
          created_at: '2026-04-01T00:00:00Z',
        },
      ],
      total: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useAdminPendingComments(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/admin/comments/pending')
    expect(url).toContain('limit=25')
    expect(url).toContain('offset=0')
    expect(result.current.data?.comments).toHaveLength(1)
    expect(result.current.data?.total).toBe(1)
  })

  it('uses custom limit and offset', async () => {
    mockApiRequest.mockResolvedValueOnce({ comments: [], total: 0 })

    const { result } = renderHook(() => useAdminPendingComments(10, 20), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('limit=10')
    expect(url).toContain('offset=20')
  })
})

describe('useAdminApproveComment', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('calls approve endpoint with comment ID', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useAdminApproveComment(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(123)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/comments/123/approve',
      { method: 'POST' }
    )
  })
})

describe('useAdminRejectComment', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('calls reject endpoint with comment ID and reason', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useAdminRejectComment(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ commentId: 456, reason: 'Spam' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/comments/456/reject',
      {
        method: 'POST',
        body: JSON.stringify({ reason: 'Spam' }),
      }
    )
  })
})

describe('useAdminHideComment', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('calls hide endpoint with comment ID and reason', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useAdminHideComment(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ commentId: 789, reason: 'Abusive' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/comments/789/hide',
      {
        method: 'POST',
        body: JSON.stringify({ reason: 'Abusive' }),
      }
    )
  })
})

describe('useAdminRestoreComment', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('calls restore endpoint with comment ID', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useAdminRestoreComment(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(101)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/comments/101/restore',
      { method: 'POST' }
    )
  })

  it('invalidates admin comments AND public comments on success', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useAdminRestoreComment(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate(101)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: adminCommentQueryKeys.all,
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['comments'],
    })
  })
})

describe('adminCommentQueryKeys', () => {
  it('generates stable keys for the moderation queue', () => {
    expect(adminCommentQueryKeys.all).toEqual(['admin', 'comments'])
    expect(adminCommentQueryKeys.pending({ limit: 25, offset: 0 })).toEqual([
      'admin',
      'comments',
      'pending',
      { limit: 25, offset: 0 },
    ])
    expect(adminCommentQueryKeys.edits(42)).toEqual([
      'admin',
      'comments',
      'edits',
      42,
    ])
  })
})

describe('invalidation on moderation mutations', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('approve invalidates admin queue AND public comments', async () => {
    // After approve the comment moves from pending → visible, so BOTH
    // the moderation queue AND any entity-detail comment list must refetch.
    mockApiRequest.mockResolvedValueOnce(undefined)

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useAdminApproveComment(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate(123)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: adminCommentQueryKeys.all,
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['comments'],
    })
  })

  it('reject invalidates admin queue AND public comments', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useAdminRejectComment(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({ commentId: 456, reason: 'spam' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: adminCommentQueryKeys.all,
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['comments'],
    })
  })

  it('hide invalidates admin queue, public comments, AND entity reports', async () => {
    // Hiding a comment usually resolves an entity report at the call site
    // (the moderation queue couples both), so the entity-reports key is
    // also invalidated so the queue clears the related row.
    mockApiRequest.mockResolvedValueOnce(undefined)

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useAdminHideComment(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate({ commentId: 789, reason: 'abusive' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: adminCommentQueryKeys.all,
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['comments'],
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['admin', 'entityReports'],
    })
  })

  it('does not invalidate when a mutation fails', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Not found'))

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useAdminApproveComment(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      result.current.mutate(999)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(invalidateSpy).not.toHaveBeenCalled()
  })
})

describe('useAdminCommentEditHistory', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches the edit history when enabled AND commentId > 0', async () => {
    const mockHistory = {
      comment_id: 42,
      current_body: 'edited body',
      edits: [
        {
          id: 1,
          comment_id: 42,
          old_body: 'original body',
          edited_at: '2026-04-01T00:00:00Z',
        },
      ],
    }
    mockApiRequest.mockResolvedValueOnce(mockHistory)

    const { result } = renderHook(
      () => useAdminCommentEditHistory(42, true),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/admin/comments/42/edits',
      { method: 'GET' }
    )
    expect(result.current.data?.edits).toHaveLength(1)
  })

  it('defaults to disabled so we do not prefetch every comment’s history', () => {
    // The hook's default `enabled=false` is load-bearing — it prevents the
    // moderation queue from prefetching every comment's history on mount.
    const { result } = renderHook(() => useAdminCommentEditHistory(42), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('does not fetch when commentId is 0 even if enabled', () => {
    const { result } = renderHook(
      () => useAdminCommentEditHistory(0, true),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })
})
