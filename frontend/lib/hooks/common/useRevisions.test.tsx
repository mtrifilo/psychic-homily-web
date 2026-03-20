import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    REVISIONS: {
      ENTITY_HISTORY: (entityType: string, entityId: string | number) =>
        `/api/revisions/${entityType}/${entityId}`,
      DETAIL: (revisionId: number) => `/api/revisions/${revisionId}`,
      USER_REVISIONS: (userId: string | number) =>
        `/api/users/${userId}/revisions`,
      ROLLBACK: (revisionId: number) =>
        `/api/admin/revisions/${revisionId}/rollback`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    revisions: {
      all: ['revisions'] as const,
      entity: (entityType: string, entityId: string | number) =>
        ['revisions', 'entity', entityType, String(entityId)] as const,
      detail: (revisionId: number) =>
        ['revisions', 'detail', revisionId] as const,
      user: (userId: string | number) =>
        ['revisions', 'user', String(userId)] as const,
    },
  },
}))

// Import hooks after mocks are set up
import {
  useEntityRevisions,
  useRevision,
  useUserRevisions,
  useRollbackRevision,
} from './useRevisions'

const mockRevisions = [
  {
    id: 1,
    entity_type: 'artist',
    entity_id: 42,
    user_id: 10,
    user_name: 'admin',
    changes: [
      { field: 'name', old_value: 'Old Name', new_value: 'New Name' },
    ],
    summary: 'Updated artist name',
    created_at: '2026-03-01T12:00:00Z',
  },
  {
    id: 2,
    entity_type: 'artist',
    entity_id: 42,
    user_id: 11,
    user_name: 'contributor',
    changes: [
      { field: 'city', old_value: 'Phoenix', new_value: 'Tempe' },
      { field: 'state', old_value: 'AZ', new_value: 'AZ' },
    ],
    summary: 'Updated location',
    created_at: '2026-03-02T14:00:00Z',
  },
]

describe('useRevisions hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('useEntityRevisions', () => {
    it('fetches revision history for an entity', async () => {
      const mockResponse = { revisions: mockRevisions, total: 2 }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useEntityRevisions('artist', 42),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/api/revisions/artist/42?limit=20'
      )
      expect(result.current.data?.revisions).toHaveLength(2)
      expect(result.current.data?.total).toBe(2)
    })

    it('accepts string entity ID', async () => {
      mockApiRequest.mockResolvedValueOnce({ revisions: [], total: 0 })

      const { result } = renderHook(
        () => useEntityRevisions('venue', 'the-rebel-lounge'),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/api/revisions/venue/the-rebel-lounge?limit=20'
      )
    })

    it('includes custom limit and offset in query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ revisions: [], total: 0 })

      const { result } = renderHook(
        () => useEntityRevisions('artist', 42, { limit: 10, offset: 5 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0] as string
      expect(calledUrl).toContain('limit=10')
      expect(calledUrl).toContain('offset=5')
    })

    it('uses default limit of 20 and offset of 0', async () => {
      mockApiRequest.mockResolvedValueOnce({ revisions: [], total: 0 })

      const { result } = renderHook(
        () => useEntityRevisions('artist', 42),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0] as string
      expect(calledUrl).toContain('limit=20')
      // offset=0 is falsy, so it won't be in the URL
      expect(calledUrl).not.toContain('offset=')
    })

    it('can be disabled via enabled option', () => {
      const { result } = renderHook(
        () => useEntityRevisions('artist', 42, { enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('is enabled by default when enabled is not specified', async () => {
      mockApiRequest.mockResolvedValueOnce({ revisions: [], total: 0 })

      const { result } = renderHook(
        () => useEntityRevisions('artist', 42),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalled()
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(
        () => useEntityRevisions('artist', 42),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect(result.current.error).toBeDefined()
    })

    it('returns revision data with field changes', async () => {
      const mockResponse = { revisions: mockRevisions, total: 2 }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(
        () => useEntityRevisions('artist', 42),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const firstRevision = result.current.data?.revisions[0]
      expect(firstRevision?.changes).toHaveLength(1)
      expect(firstRevision?.changes[0].field).toBe('name')
      expect(firstRevision?.changes[0].old_value).toBe('Old Name')
      expect(firstRevision?.changes[0].new_value).toBe('New Name')
      expect(firstRevision?.user_name).toBe('admin')
    })
  })

  describe('useRevision', () => {
    it('fetches a single revision by ID', async () => {
      const mockRevision = mockRevisions[0]
      mockApiRequest.mockResolvedValueOnce(mockRevision)

      const { result } = renderHook(() => useRevision(1), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/api/revisions/1'
      )
      expect(result.current.data?.id).toBe(1)
      expect(result.current.data?.summary).toBe('Updated artist name')
    })

    it('does not fetch when revisionId is 0', () => {
      const { result } = renderHook(() => useRevision(0), {
        wrapper: createWrapper(),
      })

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when revisionId is negative', () => {
      const { result } = renderHook(() => useRevision(-1), {
        wrapper: createWrapper(),
      })

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('can be disabled via enabled option', () => {
      const { result } = renderHook(
        () => useRevision(1, { enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles not found error', async () => {
      const error = new Error('Revision not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useRevision(999), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).message).toBe(
        'Revision not found'
      )
    })
  })

  describe('useUserRevisions', () => {
    it('fetches revisions for a specific user', async () => {
      const mockResponse = { revisions: mockRevisions, total: 2 }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useUserRevisions(10), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/api/users/10/revisions?limit=20'
      )
      expect(result.current.data?.revisions).toHaveLength(2)
    })

    it('accepts string user ID', async () => {
      mockApiRequest.mockResolvedValueOnce({ revisions: [], total: 0 })

      const { result } = renderHook(() => useUserRevisions('42'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/api/users/42/revisions?limit=20'
      )
    })

    it('includes custom limit and offset in query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ revisions: [], total: 0 })

      const { result } = renderHook(
        () => useUserRevisions(10, { limit: 5, offset: 10 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      const calledUrl = mockApiRequest.mock.calls[0][0] as string
      expect(calledUrl).toContain('limit=5')
      expect(calledUrl).toContain('offset=10')
    })

    it('can be disabled via enabled option', () => {
      const { result } = renderHook(
        () => useUserRevisions(10, { enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles API errors', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useUserRevisions(10), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useRollbackRevision', () => {
    it('sends POST to rollback a revision', async () => {
      mockApiRequest.mockResolvedValueOnce({ success: true })

      const { result } = renderHook(() => useRollbackRevision(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(1)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/api/admin/revisions/1/rollback',
        { method: 'POST' }
      )
    })

    it('invalidates revision queries on success', async () => {
      const queryClient = createTestQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      mockApiRequest.mockResolvedValueOnce({ success: true })

      const { result } = renderHook(() => useRollbackRevision(), { wrapper })

      await act(async () => {
        result.current.mutate(1)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['revisions'],
      })
    })

    it('handles rollback error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useRollbackRevision(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(999)
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).message).toBe('Forbidden')
    })

    it('does not invalidate queries on error', async () => {
      const queryClient = createTestQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      mockApiRequest.mockRejectedValueOnce(new Error('Server error'))

      const { result } = renderHook(() => useRollbackRevision(), { wrapper })

      await act(async () => {
        result.current.mutate(1)
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      // onSuccess should not have been called, so no invalidation
      expect(invalidateSpy).not.toHaveBeenCalledWith({
        queryKey: ['revisions'],
      })
    })
  })
})
