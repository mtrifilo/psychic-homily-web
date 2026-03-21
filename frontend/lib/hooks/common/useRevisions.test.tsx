import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    REVISIONS: {
      ENTITY_HISTORY: (entityType: string, entityId: string | number) =>
        `/revisions/${entityType}/${entityId}`,
      DETAIL: (revisionId: number) => `/revisions/${revisionId}`,
      USER_REVISIONS: (userId: string | number) => `/users/${userId}/revisions`,
      ROLLBACK: (revisionId: number) => `/admin/revisions/${revisionId}/rollback`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    revisions: {
      all: ['revisions'],
      entity: (entityType: string, entityId: string | number) =>
        ['revisions', 'entity', entityType, String(entityId)],
      detail: (revisionId: number) => ['revisions', 'detail', revisionId],
      user: (userId: string | number) => ['revisions', 'user', String(userId)],
    },
  },
}))

import {
  useEntityRevisions,
  useRevision,
  useUserRevisions,
  useRollbackRevision,
} from './useRevisions'


describe('useEntityRevisions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches revisions for an entity', async () => {
    mockApiRequest.mockResolvedValueOnce({
      revisions: [
        { id: 1, entity_type: 'artist', entity_id: 42, changes: [] },
      ],
      total: 1,
    })

    const { result } = renderHook(
      () => useEntityRevisions('artist', 42),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/revisions/artist/42')
    expect(url).toContain('limit=20')
    // Note: offset=0 is falsy, so the hook's `if (offset)` check skips it.
    // This is a minor bug -- offset 0 is valid but gets omitted.
    // The backend defaults to 0 anyway, so it's functionally correct.
  })

  it('includes custom limit and offset', async () => {
    mockApiRequest.mockResolvedValueOnce({ revisions: [], total: 0 })

    const { result } = renderHook(
      () => useEntityRevisions('venue', 10, { limit: 50, offset: 20 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('limit=50')
    expect(url).toContain('offset=20')
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(
      () => useEntityRevisions('artist', 42, { enabled: false }),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useRevision', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches a single revision', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 1,
      entity_type: 'artist',
      entity_id: 42,
      changes: [{ field: 'name', old_value: 'Old', new_value: 'New' }],
    })

    const { result } = renderHook(() => useRevision(1), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/revisions/1')
  })

  it('does not fetch when revisionId is 0', () => {
    const { result } = renderHook(() => useRevision(0), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(() => useRevision(1, { enabled: false }), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useUserRevisions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches revisions for a user', async () => {
    mockApiRequest.mockResolvedValueOnce({ revisions: [], total: 0 })

    const { result } = renderHook(() => useUserRevisions(5), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/users/5/revisions')
    expect(url).toContain('limit=20')
  })
})

describe('useRollbackRevision', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('rolls back a revision with POST', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useRollbackRevision(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/revisions/1/rollback',
      expect.objectContaining({ method: 'POST' })
    )
  })

  it('handles rollback errors', async () => {
    const error = new Error('Forbidden')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useRollbackRevision(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})
