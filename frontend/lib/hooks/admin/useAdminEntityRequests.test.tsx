import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()
const mockInvalidateAdminEntityRequests = vi.fn()

// `isConflictError` is deliberately NOT stubbed: the 409 rule the mutations
// branch on is the shipped one, not a second copy written for this file.
vi.mock('@/lib/api', async importOriginal => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    isConflictError: actual.isConflictError,
    apiRequest: (...args: unknown[]) => mockApiRequest(...args),
    API_ENDPOINTS: {
      ADMIN: {
        ENTITY_REQUESTS: {
          LIST: '/admin/entity-requests',
          DECIDE: (id: string | number) => `/admin/entity-requests/${id}/decide`,
          FULFILL: (id: string | number) => `/admin/entity-requests/${id}/fulfill`,
        },
      },
    },
    API_BASE_URL: 'http://localhost:8080',
  }
})

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    admin: {
      entityRequests: (params: Record<string, unknown>) => ['admin', 'entityRequests', params],
    },
  },
  createInvalidateQueries: () => ({
    adminEntityRequests: mockInvalidateAdminEntityRequests,
    artists: vi.fn(),
    venues: vi.fn(),
    labels: vi.fn(),
    releases: vi.fn(),
    festivals: vi.fn(),
    shows: vi.fn(),
  }),
}))

import { useDecideEntityRequest, useRescueEntityRequest } from './useAdminEntityRequests'

/** The body `apiRequest` was called with, parsed back out of the request init. */
function sentBody(): Record<string, unknown> {
  const [, init] = mockApiRequest.mock.calls[0] as [string, RequestInit]
  return JSON.parse(init.body as string)
}

/** An error shaped the way `apiRequest` throws one. */
function apiError(status: number, message = 'failed'): Error & { status: number } {
  return Object.assign(new Error(message), { status })
}

describe('useDecideEntityRequest', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  // PSY-1974. The version is a STRING on the wire and must round-trip byte for
  // byte: a timestamptz holds microseconds that a JavaScript Date does not, so a
  // value re-serialized through one would 409 every decision.
  it('sends expected_updated_at verbatim, microseconds and all', async () => {
    mockApiRequest.mockResolvedValue({})
    const { result } = renderHook(() => useDecideEntityRequest(), { wrapper: createWrapper() })

    await act(async () => {
      result.current.mutate({
        id: 9,
        decision: 'approved',
        expected_updated_at: '2026-09-03T02:03:04.123456Z',
      })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/entity-requests/9/decide',
      expect.objectContaining({ method: 'POST' })
    )
    expect(sentBody().expected_updated_at).toBe('2026-09-03T02:03:04.123456Z')
  })

  // Omitting it is a supported call: the endpoint then decides against whatever
  // the row currently holds, which is the pre-PSY-1974 contract.
  it('omits the key entirely when no version is supplied', async () => {
    mockApiRequest.mockResolvedValue({})
    const { result } = renderHook(() => useDecideEntityRequest(), { wrapper: createWrapper() })

    await act(async () => {
      result.current.mutate({ id: 9, decision: 'rejected', note: 'not notable' })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(sentBody()).not.toHaveProperty('expected_updated_at')
  })

  it('refetches the queue when the decision conflicts', async () => {
    mockApiRequest.mockRejectedValue(apiError(409, 'was revised by its requester'))
    const { result } = renderHook(() => useDecideEntityRequest(), { wrapper: createWrapper() })

    await act(async () => {
      result.current.mutate({ id: 9, decision: 'approved', expected_updated_at: 'v1' })
    })
    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(mockInvalidateAdminEntityRequests).toHaveBeenCalled()
  })

  // A 422 says the admin's own input was wrong, not that the row moved.
  // Refetching there would discard nothing but would also assert something
  // untrue on the card.
  it('does not refetch for a failure that is not a conflict', async () => {
    mockApiRequest.mockRejectedValue(apiError(422, 'Approving a show requires show_venue'))
    const { result } = renderHook(() => useDecideEntityRequest(), { wrapper: createWrapper() })

    await act(async () => {
      result.current.mutate({ id: 9, decision: 'approved' })
    })
    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(mockInvalidateAdminEntityRequests).not.toHaveBeenCalled()
  })
})

describe('useRescueEntityRequest', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('refetches the queue when a rescue conflicts', async () => {
    mockApiRequest.mockRejectedValue(apiError(409, 'already fulfilled by a concurrent rescue'))
    const { result } = renderHook(() => useRescueEntityRequest(), { wrapper: createWrapper() })

    await act(async () => {
      result.current.mutate({ id: 4, action: 'fulfill' })
    })
    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(mockInvalidateAdminEntityRequests).toHaveBeenCalled()
  })

  // The rescue endpoint takes no version: only a PENDING row's submission is
  // ever replaced, and every row this acts on is approved.
  it('never sends a version', async () => {
    mockApiRequest.mockResolvedValue({})
    const { result } = renderHook(() => useRescueEntityRequest(), { wrapper: createWrapper() })

    await act(async () => {
      result.current.mutate({ id: 4, action: 'void', note: 'orphan' })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(sentBody()).not.toHaveProperty('expected_updated_at')
  })
})
