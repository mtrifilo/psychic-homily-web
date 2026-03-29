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

})
