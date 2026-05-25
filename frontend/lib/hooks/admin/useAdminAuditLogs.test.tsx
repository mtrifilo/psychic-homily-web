import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      AUDIT_LOGS: {
        LIST: '/admin/audit-logs',
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    admin: {
      auditLogs: (limit: number, offset: number) =>
        ['admin', 'auditLogs', { limit, offset }],
    },
  },
}))

import { useAuditLogs } from './useAdminAuditLogs'


describe('useAuditLogs', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches audit logs with default pagination', async () => {
    const mockLogs = {
      logs: [
        { id: 1, action: 'show.approved', created_at: '2025-03-17T12:00:00Z' },
      ],
      total: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockLogs)

    const { result } = renderHook(() => useAuditLogs(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/admin/audit-logs')
    expect(url).toContain('limit=50')
    expect(url).toContain('offset=0')
  })

  it('uses custom limit and offset', async () => {
    mockApiRequest.mockResolvedValueOnce({ logs: [], total: 0 })

    const { result } = renderHook(() => useAuditLogs({ limit: 20, offset: 40 }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('limit=20')
    expect(url).toContain('offset=40')
  })

  it('starts in loading state and resolves to success', async () => {
    // Hook contract: useQuery never blocks the initial render. We render
    // first, observe the loading state, then let the promise resolve.
    let resolveFetch: (value: unknown) => void = () => {}
    const pending = new Promise((resolve) => {
      resolveFetch = resolve
    })
    mockApiRequest.mockReturnValueOnce(pending)

    const { result } = renderHook(() => useAuditLogs(), {
      wrapper: createWrapper(),
    })

    expect(result.current.isLoading).toBe(true)
    expect(result.current.data).toBeUndefined()

    resolveFetch({ logs: [], total: 0 })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.isLoading).toBe(false)
  })

  it('surfaces fetch errors', async () => {
    const error = new Error('Forbidden')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useAuditLogs(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Forbidden')
    expect(result.current.data).toBeUndefined()
  })

  it('keys cache by limit AND offset', async () => {
    // Two different paginations must produce two distinct query keys,
    // not collide and skip the second fetch.
    mockApiRequest
      .mockResolvedValueOnce({ logs: [], total: 0 })
      .mockResolvedValueOnce({ logs: [], total: 0 })

    const { result: page1 } = renderHook(
      () => useAuditLogs({ limit: 50, offset: 0 }),
      { wrapper: createWrapper() }
    )
    await waitFor(() => expect(page1.current.isSuccess).toBe(true))

    const { result: page2 } = renderHook(
      () => useAuditLogs({ limit: 50, offset: 50 }),
      { wrapper: createWrapper() }
    )
    await waitFor(() => expect(page2.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledTimes(2)
  })
})
