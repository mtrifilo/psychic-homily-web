import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

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
})
