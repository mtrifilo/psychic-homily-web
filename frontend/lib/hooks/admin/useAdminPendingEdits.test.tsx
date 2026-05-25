import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()
const mockInvalidateAdminPendingEdits = vi.fn()
const mockInvalidateArtists = vi.fn()
const mockInvalidateVenues = vi.fn()
const mockInvalidateFestivals = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      PENDING_EDITS: {
        LIST: '/admin/pending-edits',
        APPROVE: (editId: string | number) =>
          `/admin/pending-edits/${editId}/approve`,
        REJECT: (editId: string | number) =>
          `/admin/pending-edits/${editId}/reject`,
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    admin: {
      pendingEdits: (params: Record<string, unknown>) =>
        ['admin', 'pendingEdits', params],
    },
  },
  createInvalidateQueries: () => ({
    adminPendingEdits: mockInvalidateAdminPendingEdits,
    artists: mockInvalidateArtists,
    venues: mockInvalidateVenues,
    festivals: mockInvalidateFestivals,
  }),
}))

import {
  useAdminPendingEdits,
  useApprovePendingEdit,
  useRejectPendingEdit,
} from './useAdminPendingEdits'

describe('useAdminPendingEdits', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches pending edits with default filters', async () => {
    const mockResponse = {
      edits: [
        {
          id: 1,
          entity_type: 'artist',
          entity_id: 42,
          submitted_by: 7,
          submitter_username: 'alice',
          field_changes: [],
          summary: 'Fix typo',
          status: 'pending',
          created_at: '2026-04-01T00:00:00Z',
          updated_at: '2026-04-01T00:00:00Z',
        },
      ],
      total: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useAdminPendingEdits(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/admin/pending-edits')
    expect(url).toContain('status=pending')
    expect(url).toContain('limit=50')
    expect(url).toContain('offset=0')
    expect(url).not.toContain('entity_type=')
    expect(result.current.data?.edits).toHaveLength(1)
  })

  it('passes entity_type and pagination filters', async () => {
    mockApiRequest.mockResolvedValueOnce({ edits: [], total: 0 })

    const { result } = renderHook(
      () =>
        useAdminPendingEdits({
          status: 'approved',
          entity_type: 'venue',
          limit: 10,
          offset: 30,
        }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('status=approved')
    expect(url).toContain('entity_type=venue')
    expect(url).toContain('limit=10')
    expect(url).toContain('offset=30')
  })

  it('omits status param when status is explicitly empty', async () => {
    mockApiRequest.mockResolvedValueOnce({ edits: [], total: 0 })

    const { result } = renderHook(
      () => useAdminPendingEdits({ status: '' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).not.toContain('status=')
  })

  it('surfaces fetch errors', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Forbidden'))

    const { result } = renderHook(() => useAdminPendingEdits(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Forbidden')
  })
})

describe('useApprovePendingEdit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateAdminPendingEdits.mockReset()
    mockInvalidateArtists.mockReset()
    mockInvalidateVenues.mockReset()
    mockInvalidateFestivals.mockReset()
  })

  it('POSTs to the approve endpoint with the edit id', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, status: 'approved' })

    const { result } = renderHook(() => useApprovePendingEdit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/pending-edits/1/approve',
      { method: 'POST' }
    )
  })

  it('invalidates pending-edits AND each entity surface on success', async () => {
    // Approve mutates the underlying entity, so the hook fans out
    // invalidation across artists/venues/festivals plus the queue itself.
    // We don't know the entity_type at mutation time (the API call only
    // takes id), so fanning out is the safe contract — guard it.
    mockApiRequest.mockResolvedValueOnce({ id: 1 })

    const { result } = renderHook(() => useApprovePendingEdit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateAdminPendingEdits).toHaveBeenCalledTimes(1)
    expect(mockInvalidateArtists).toHaveBeenCalledTimes(1)
    expect(mockInvalidateVenues).toHaveBeenCalledTimes(1)
    expect(mockInvalidateFestivals).toHaveBeenCalledTimes(1)
  })

  it('does not invalidate on failure', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Conflict'))

    const { result } = renderHook(() => useApprovePendingEdit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(999)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Conflict')
    expect(mockInvalidateAdminPendingEdits).not.toHaveBeenCalled()
    expect(mockInvalidateArtists).not.toHaveBeenCalled()
  })
})

describe('useRejectPendingEdit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateAdminPendingEdits.mockReset()
    mockInvalidateArtists.mockReset()
    mockInvalidateVenues.mockReset()
    mockInvalidateFestivals.mockReset()
  })

  it('POSTs to the reject endpoint with the rejection reason', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, status: 'rejected' })

    const { result } = renderHook(() => useRejectPendingEdit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ editId: 1, reason: 'Inaccurate' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/pending-edits/1/reject',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ reason: 'Inaccurate' }),
      })
    )
  })

  it('invalidates only pending-edits on reject (not entity surfaces)', async () => {
    // Reject doesn't mutate the entity itself — only the queue state
    // changes. Guard that the entity invalidations are NOT triggered so
    // we don't waste round-trips refetching unaffected detail pages.
    mockApiRequest.mockResolvedValueOnce({ id: 1 })

    const { result } = renderHook(() => useRejectPendingEdit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ editId: 1, reason: 'Spam' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateAdminPendingEdits).toHaveBeenCalledTimes(1)
    expect(mockInvalidateArtists).not.toHaveBeenCalled()
    expect(mockInvalidateVenues).not.toHaveBeenCalled()
    expect(mockInvalidateFestivals).not.toHaveBeenCalled()
  })

  it('surfaces mutation errors', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Not found'))

    const { result } = renderHook(() => useRejectPendingEdit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ editId: 999, reason: 'Bad' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Not found')
  })
})
